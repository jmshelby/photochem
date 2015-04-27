package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jmshelby/photochem/home"
)

const (
	DefaultWaitTime        = 2000
	DefaultNumberOfWorkers = 3
)

var startUri string
var originalHost string
var homeDb *home.DB
var GlobalWG sync.WaitGroup

func main() {

	initDestruct()

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

	var numberOfWorkers int
	if len(args) > 3 {
		var convErr error
		numberOfWorkers, convErr = strconv.Atoi(args[3])
		if convErr != nil {
			fmt.Printf("Bad number of workers param")
			os.Exit(2)
		}
	} else {
		numberOfWorkers = DefaultNumberOfWorkers
	}

	var waitTime int
	if len(args) > 4 {
		var convErr error
		waitTime, convErr = strconv.Atoi(args[4])
		if convErr != nil {
			fmt.Printf("Bad param for millisecond wait time")
			os.Exit(2)
		}
	} else {
		waitTime = DefaultWaitTime
	}

	unlocked := acquireLock(9293)
	if !unlocked {
		fmt.Println("Process already running")
		os.Exit(0)
	}

	// Start Up access to our listings
	homeDb = home.NewDB(dbHost, dbName)

	// Make a channel to pass new interesting links
	linkQueue := make(chan string, 100)
	// Make a regular queue for non-listing pages
	pageQueue := make(chan string, numberOfWorkers)
	// Make a prioritized queue for listings
	listingQueue := make(chan string, numberOfWorkers)

	// Start up workers
	for i := 0; i < numberOfWorkers; i++ {
		fmt.Println("Staring up worker ", i+1)
		go queueWorker(pageQueue, listingQueue, linkQueue, waitTime, i+1)
		GlobalWG.Add(1)
	}

	// Start link router
	go queueRouter(linkQueue, listingQueue, pageQueue)
	GlobalWG.Add(1)

	// Prime with starting uri
	pageQueue <- startUri

	// Wait till they finish (which will probably never happen)
	GlobalWG.Wait()

	// Cleanup
	cleanup()
}

func cleanup() {

	// TODO - close channels

	// Call cleanup on our db instance
	if homeDb != nil {
		homeDb.Cleanup()
	}
}

func initDestruct() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Printf("Caught Interupt Signal, cleaning up, and exiting...\n")
		cleanup()
		os.Exit(1)
	}()
}

func queueRouter(linkQueue, listingQueue, pageQueue chan string) {

	defer GlobalWG.Done()

	var pagesQueued = make(map[string]bool)

	for link := range linkQueue {

		// Skip if we've seen it before
		if pagesQueued[link] {
			continue
		}
		// Remember that we saw this link
		pagesQueued[link] = true

		// Route
		if isListingUri(link) {
			if doesListingExist(link) {
				fmt.Println("Listing already exists, skipping: ", link)
			} else {
				go func(listingLink string) { listingQueue <- listingLink }(link)
			}
		} else {
			go func(pageLink string) { pageQueue <- pageLink }(link)
		}

	}
}

func queueWorker(queue, priorityQueue, linkQueue chan string, delay int, workerNumber int) {

	defer GlobalWG.Done()

	// Start with an entry from the main queue
	for uri := range queue {

		// Chew through listing queue as higher priority
		for {
			select {
			case listingUri := <-priorityQueue:
				crawl(listingUri, linkQueue)
				time.Sleep(time.Duration(delay) * time.Millisecond)
				continue
			default:
			}
			break
		}

		crawl(uri, linkQueue)

		time.Sleep(time.Duration(delay) * time.Millisecond)
	}
}

func crawl(uri string, linkQueue chan string) {

	fmt.Println("Fetching: ", uri)
	body := fetchPage(uri)

	if body == nil {
		fmt.Println("Problem Fetching, skipping ...")
		return
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	bodyString := buf.String()

	body.Close()

	// Register Listing in our database, if its a listing
	registerListing(uri, bodyString)

	// Pull out potential new links
	links := collectInterestingLinks(bodyString)

	for _, link := range links {
		absolute := resolveReferenceLink(link, uri)
		if absolute != "" {
			if shouldAddToQueue(absolute) {
				// Pass to queue to be routed/prioritized
				linkQueue <- absolute
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

	// No if it's related to rent listings
	if match, _ := regexp.MatchString("/(rentals|off-campus-housing)/", strings.ToLower(uri)); match {
		return false
	}

	// TEMP - If this is a listing uri
	if isListingUri(uri) {
		// No if it's not in a couple of states
		if match, _ := regexp.MatchString("-(co|ca|ny|hi)-", strings.ToLower(uri)); !match {
			return false
		}
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

// History

func markPageVisited(uri string) {
	homeDb.MarkPageVisited(uri)
}

func wasPageVisited(uri string) bool {
	return homeDb.WasPageVisited(uri)
}

// Queue Tracking

func isPageQueued(uri string) bool {
	return homeDb.IsPageQueued(uri)
}

func queuePage(uri string) {
	homeDb.QueuePage(uri)
}

func deQueuePage(uri string) {
	homeDb.DeQueuePage(uri)
}

// Listings
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

	//fmt.Printf("Listing Registered: %+v\n", listing)
	fmt.Printf("Listing Registered: (%v) %v\n", listing.Id.Hex(), uri)

}

// Process Level Lock

func acquireLock(lockId int) bool {

	// Hacky way to lock this process down to only one process
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", lockId))
	if err != nil {
		if strings.Index(err.Error(), "in use") != -1 {
			return false
		} else {
			panic(err)
		}
	}

	// TODO - anything else we need to do to keep this open??
	go func() {
		listener.Accept()
	}()

	return true
}
