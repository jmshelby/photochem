package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmshelby/photochem/home"
)

const (
	DefaultNumberOfWorkers = 3
)

var GlobalWG sync.WaitGroup

func main() {

	fmt.Printf("Started - %v\n", time.Now())

	flag.Parse()
	args := flag.Args()
	fmt.Println(args)

	if len(args) < 1 {
		fmt.Println("Please specify db host")
		os.Exit(1)
	}
	if len(args) < 2 {
		fmt.Println("Please specify db name")
		os.Exit(1)
	}
	if len(args) < 3 {
		fmt.Println("Please specify stale days")
		os.Exit(1)
	}
	if len(args) < 4 {
		fmt.Println("Please specify batch amount")
		os.Exit(1)
	}

	dbHost := args[0]
	dbName := args[1]

	var staleDays int
	staleDays, convErr := strconv.Atoi(args[2])
	if convErr != nil {
		fmt.Printf("Bad number for stale days param")
		os.Exit(2)
	}
	staleDate := time.Now().AddDate(0, 0, -1*staleDays)

	var batchAmount int
	batchAmount, convErr = strconv.Atoi(args[3])
	if convErr != nil {
		fmt.Printf("Bad number for batch amount param")
		os.Exit(2)
	}

	var numberOfWorkers int
	if len(args) > 4 {
		var convErr error
		numberOfWorkers, convErr = strconv.Atoi(args[4])
		if convErr != nil {
			fmt.Printf("Bad number of workers param")
			os.Exit(2)
		}
	} else {
		numberOfWorkers = DefaultNumberOfWorkers
	}

	unlocked := acquireLock(9292)
	if !unlocked {
		fmt.Println("Process already running")
		os.Exit(0)
	}

	// Start Up access to our listings
	var homeDb *home.DB
	homeDb = home.NewDB(dbHost, dbName)

	// Make channel, buffered with expected number of workers
	listingQueue := make(chan home.Listing, numberOfWorkers)

	// Start up workers
	for i := 0; i < numberOfWorkers; i++ {
		fmt.Println("Starting up worker ", i+1)
		go queueWorker(listingQueue, homeDb)
	}
	GlobalWG.Add(numberOfWorkers)

	var count int = 0
	homeDb.IterateListingsOlderThan(staleDate, batchAmount, func(listing home.Listing, db *home.DB) {
		listingQueue <- listing
		count++
	})
	fmt.Printf("Finished Queing up: %v listings\n", count)

	// Close Queue
	close(listingQueue)

	GlobalWG.Wait()

	fmt.Printf("Done - %v\n", time.Now())

}

func queueWorker(queue <-chan home.Listing, db *home.DB) {
	defer GlobalWG.Done()
	for listing := range queue {
		updateListing(listing, db)
	}
}

func updateListing(listing home.Listing, db *home.DB) {

	// Fetch page
	fmt.Println("Making request...")
	bodyString, fetchErr := fetchPageString(listing.Url)
	fmt.Println("Making request...Done")
	if fetchErr != nil {
		fmt.Printf("Error Fetching Markup\n  -> %s\n  ==> %s\n", listing.Url, fetchErr)
		// TODO -- should we retry on errors??
		return
	}

	// Re-Register with our home db
	listing, existed, err := db.RegisterListing(listing.Url, listing.Source, bodyString)
	if err != nil {
		fmt.Printf("[ERR] Problem re-registering listing: %s - %s\n", listing.Url, err)
		return
	}

	if !existed {
		fmt.Printf("[INFO] Somehow listing did not exist, and was inserted: %s - %s\n", listing.Url, listing.Id)
	}

	fmt.Printf("Listing Registered: %+v\n", listing)
	//fmt.Printf("Listing Registered: (%v) %v\n", listing.Id.Hex(), listing.Url)

}

func fetchPageString(uri string) (string, error) {
	body, err := fetchPage(uri)

	if err != nil {
		return "", err
	}

	defer body.Close()
	buf := new(bytes.Buffer)
	buf.ReadFrom(body)
	return buf.String(), nil
}

func fetchPage(uri string) (io.ReadCloser, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := http.Client{Transport: transport}

	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 6.3; Trident/7.0; rv:11.0) like Gecko")

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

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

// ajax image request example
// http://www.zillow.com/AjaxRender.htm?encparams=9~646157445473039082~CB_-1qRS8CEVNENXoac54dVO6bpQ9JXPqUZQbgBILh8zxZXnO5NWnbZAECg2MhZm7uGut55uir5fNiq3HD0xFN3IJgW48jmzWCnqtH40wSQ5J-n4oSbY_7DOmv61BMVQ4hzXJ0a7oqNjRHLet28PkKEaLs_1uSSusypJdBvpTReWTDJl7HjrFxk2lvx7R_MB
