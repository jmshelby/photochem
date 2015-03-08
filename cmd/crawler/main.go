package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	_ "time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jmshelby/photochem/home"
)

func main() {

	flag.Parse()
	args := flag.Args()
	fmt.Println(args)
	if len(args) < 1 {
		fmt.Println("Please specify start page")
		os.Exit(1)
	}

	// TODO -- add arg for pattern matching?? (or just use hostname?)
	startUri = args[0]

	startUrl, _ := url.Parse(startUri)
	originalHost = startUrl.Host

	// Start Up access to our listings
	homeDb = home.NewDB("hqopti1", "PhotoChem-dev")

	pageQueue := make(chan string)
	listingQueue := make(chan string)

	go func() { pageQueue <- startUri }()

	for uri := range pageQueue {
		// Chew through listing queue as higher priority
		for {
			select {
			case listingUri := <-listingQueue:
				crawl(listingUri, pageQueue, listingQueue)
				continue
			default:
			}
			break
		}

		crawl(uri, pageQueue, listingQueue)
		//time.Sleep(500 * time.Millisecond)
	}
}

var startUri string
var originalHost string
var pagesFetched = make(map[string]bool)
var pagesQueued = make(map[string]bool)
var homeDb *home.DB

func crawl(uri string, pageQueue, listingQueue chan string) {

	fmt.Println("Fetching: ", uri)

	body := fetchPage(uri)
	pagesFetched[uri] = true
	delete(pagesQueued, uri)

	if body == nil {
		fmt.Println("Problem Fetching, skipping ...")
		return
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	bodyString := buf.String()

	body.Close()

	// Pull out images, if this is a home for sale link
	checkForAndStoreImages(uri, bodyString)

	// Pull out potential new links
	links := collectInterestingLinks(bodyString)

	for _, link := range links {
		absolute := resolveReferenceLink(link, uri)
		if uri != "" {

			if pagesQueued[absolute] {
				continue
			}

			if !pagesFetched[absolute] && shouldAddToQueue(absolute) {
				//fmt.Println("		-> Adding: ", absolute);
				if isListingUri(absolute) {
					if homeDb.DoesListingExist(absolute) {
						fmt.Println("Listing already exists, skipping: ", absolute)
					} else {
						go func() { listingQueue <- absolute }()
					}
				} else {
					go func() { pageQueue <- absolute }()
				}
				pagesQueued[absolute] = true
				//fmt.Println(absolute)
			}
		}
	}

}

func isListingUri(uri string) bool {
	// Make sure this is a property URL
	if match, _ := regexp.MatchString("www.homes.com/property/\\d", uri); !match {
		//fmt.Println("Url not a property, skipping..", uri)
		return false
	}
	return true
}

func checkForAndStoreImages(uri, pageSource string) {

	//fmt.Println("Parsing Images for => ", uri)
	if !isListingUri(uri) {
		return
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageSource))
	if err != nil {
		fmt.Printf("[ERR] Couldn't parse page: %s - %s\n", uri, err)
		return
	}

	titleText := doc.Find("title").First().Text()
	if !strings.Contains(titleText, " for sale |") {
		fmt.Println("House Not for sale: ", titleText)
		return
	}

	imageLinks := make([]string, 0)
	doc.Find("#slider img").Each(func(i int, s *goquery.Selection) {

		imageLink, _ := s.Attr("src")

		//fmt.Println("		-> ", imageLink)

		if imageLink != "" {
			imageLinks = append(imageLinks, imageLink)
		}
	})
	//fmt.Println("		--- ")
	//fmt.Println("")

	if len(imageLinks) > 0 {
		go persistImages(uri, imageLinks)
	}
}

func collectInterestingLinks(pageSource string) []string {

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageSource))
	if err != nil {
		fmt.Printf("[ERR] Couldn't parse page to collect links: %s\n", err)
		return nil
	}

	excludeSelection := doc.Find("*[class*=OffMarket] a") // All off market listings

	links := make([]string, 0)
	doc.Find("a").NotSelection(excludeSelection).Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		if link != "" {
			links = append(links, link)
		}
	})

	return links
}

func shouldAddToQueue(uri string) bool {

	// Parse Url
	url, err := url.Parse(uri)
	if err != nil {
		fmt.Println("Could not parse uri: ", uri, " error: ", err)
		return false
	}

	// No if the host is different from the starting host
	if url.Host != originalHost {
		return false
	}

	return true
}

func fetchPage(uri string) io.ReadCloser {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := http.Client{Transport: transport}

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.3; Trident/7.0; rv:11.0) like Gecko")

	resp, err := client.Do(req)

	if err != nil {
		return nil
	}
	return resp.Body
}

func resolveReferenceLink(href, base string) string {
	uri, err := url.Parse(href)
	if err != nil {
		return ""
	}
	baseUrl, err := url.Parse(base)
	if err != nil {
		return ""
	}
	uri = baseUrl.ResolveReference(uri)

	// Also clean it up ..
	// Remove fragment
	uri.Fragment = ""

	// TODO -- remove trailing slashes?

	return uri.String()
}

func persistImages(uri string, imageLinks []string) {

	listing := home.Listing{
		Url:       uri,
		Source:    "homes.com",
		ImageUrls: imageLinks,
	}

	mongoErr := getMongoCollection().Insert(record)
	if mongoErr != nil {
		fmt.Printf("Mongo Processed url: with error: %s\n", uri, mongoErr)
	} else {
		//fmt.Println("Inserted record: ", record);
		fmt.Println("Saved Images: ", uri)
	}
}

// ajax image request example
// http://www.zillow.com/AjaxRender.htm?encparams=9~646157445473039082~CB_-1qRS8CEVNENXoac54dVO6bpQ9JXPqUZQbgBILh8zxZXnO5NWnbZAECg2MhZm7uGut55uir5fNiq3HD0xFN3IJgW48jmzWCnqtH40wSQ5J-n4oSbY_7DOmv61BMVQ4hzXJ0a7oqNjRHLet28PkKEaLs_1uSSusypJdBvpTReWTDJl7HjrFxk2lvx7R_MB
