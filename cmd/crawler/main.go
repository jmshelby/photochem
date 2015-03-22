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
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jmshelby/photochem/home"
)

const (
	DefaultWaitTime = 2000
)

var startUri string
var originalHost string
var pagesFetched = make(map[string]bool)
var pagesQueued = make(map[string]bool)
var homeDb *home.DB

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

	if len(args) < 2 {
		fmt.Println("Please specify db host")
		os.Exit(1)
	}
	if len(args) < 3 {
		fmt.Println("Please specify db name")
		os.Exit(1)
	}

	dbHost := args[1]
	dbName := args[2]

	var waitTime int
	if len(args) > 3 {
		var convErr error
		waitTime, convErr = strconv.Atoi(args[3])
		if convErr != nil {
			fmt.Printf("Bad param for millisecond wait time")
			os.Exit(2)
		}
	} else {
		waitTime = DefaultWaitTime
	}

	// Start Up access to our listings
	homeDb = home.NewDB(dbHost, dbName)

	pageQueue := make(chan string)
	listingQueue := make(chan string)

	go func() { pageQueue <- startUri }()

	for uri := range pageQueue {

		// Chew through listing queue as higher priority
		for {
			select {
			case listingUri := <-listingQueue:
				crawl(listingUri, pageQueue, listingQueue)
				time.Sleep(time.Duration(waitTime) * time.Millisecond)
				continue
			default:
			}
			break
		}

		crawl(uri, pageQueue, listingQueue)

		time.Sleep(time.Duration(waitTime) * time.Millisecond)

	}
}

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
	registerListing(uri, bodyString)

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
					if doesListingExist(absolute) {
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

func isListingUri(uri string) bool {
	// Make sure this is a property URL
	if match, _ := regexp.MatchString("www.homes.com/property/\\d", uri); !match {
		//fmt.Println("Url not a property, skipping..", uri)
		return false
	}
	return true
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

// Database Stuff

func doesListingExist(uri string) bool {
	_, found := homeDb.GetListingIdFromUrl(uri)
	return (found == nil)
}

func registerListing(uri, pageSource string) {

	//fmt.Println("Parsing Images for => ", uri)
	if !isListingUri(uri) {
		return
	}

	// Register with our home db
	listing, existed, err := homeDb.RegisterListing(uri, "homes.com", pageSource)
	if err != nil {
		fmt.Printf("[ERR] Problem registering listing: %s - %s\n", uri, err)
		return
	}

	if existed {
		fmt.Printf("[INFO] Somehow listing already existed and was updated: %s - %s\n", uri, listing.Id)
	}

	fmt.Printf("Listing Registered: %+v\n", listing)

}
