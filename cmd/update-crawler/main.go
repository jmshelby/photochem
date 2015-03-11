package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/PuerkitoBio/goquery"
	"github.com/jmshelby/photochem/home"
)

const (
	MarkupExpirationDays   = 7
	DefaultNumberOfWorkers = 3
)

func main() {

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

	dbHost := args[0]
	dbName := args[1]

	var numberOfWorkers int
	if len(args) > 2 {
		var convErr error
		numberOfWorkers, convErr = strconv.Atoi(args[2])
		if convErr != nil {
			fmt.Printf("Bad number of workers param")
			os.Exit(2)
		}
	} else {
		numberOfWorkers = DefaultNumberOfWorkers
	}

	// Start Up access to our listings
	var homeDb *home.DB
	homeDb = home.NewDB(dbHost, dbName)

	fmt.Println("We have our home database, now we're gonna iterate..")

	// Make channel, buffered with expected number of workers
	listingQueue := make(chan home.Listing, numberOfWorkers)

	// Start up workers
	for i := 0; i < numberOfWorkers; i++ {
		fmt.Println("Starting up worker ", i+1)
		go queueWorker(listingQueue, homeDb)
	}

	var count int
	for {
		count = 0
		homeDb.IterateAllListings(func(listing home.Listing, db *home.DB) {
			listingQueue <- listing
			count++
		})
		fmt.Println("Somehow we're done with all of our listings... starting over again.")
		fmt.Println("Processed this many: ", count)
	}


	// Close Queue
	close(listingQueue)

}

func queueWorker(queue <-chan home.Listing, db *home.DB) {
	for listing := range queue {
		checkAndUpdateMarkup(listing, db)
	}
}

func checkAndUpdateMarkup(listing home.Listing, db *home.DB) {

	// Get the latest date of markup for listing
	date, found := db.GetNewestMarkupDate(listing.Id)

	// If found and date is not expired, don't continue
	if found && !isMarkupExpired(date) {
		//fmt.Println("Current markup record is not expired yet, skipping\n  ->", listing.Url)
		return
	}

	// Fetch page
	//fmt.Println("Making request...")
	bodyString, fetchErr := fetchPageString(listing.Url)
	//fmt.Println("Making request...Done")
	if fetchErr != nil {
		fmt.Printf("Error Fetching Markup\n  -> %s\n  ==> %s\n", listing.Url, fetchErr)
		// TODO -- should we retry on errors??
		return
	}

	// TODO -- is this safe??
	go func() {

		// Save Markup
		saveErr := db.SaveMarkup(listing.Id, listing.Url, listing.Source, bodyString)

		if saveErr == nil {
			bytes := len(bodyString)
			fmt.Printf("Saved Markup (%v bytes)\n  -> %s\n", bytes, listing.Url)
		} else {
			fmt.Printf("Error Saving Markup\n  -> %s\n  ==> %s\n", listing.Url, saveErr)
			return
			// TODO -- should we retry on errors??
		}

	}()

}

func isMarkupExpired(date time.Time) bool {
	expirationHours := float64(MarkupExpirationDays * 24)
	return time.Since(date).Hours() > expirationHours
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

// ajax image request example
// http://www.zillow.com/AjaxRender.htm?encparams=9~646157445473039082~CB_-1qRS8CEVNENXoac54dVO6bpQ9JXPqUZQbgBILh8zxZXnO5NWnbZAECg2MhZm7uGut55uir5fNiq3HD0xFN3IJgW48jmzWCnqtH40wSQ5J-n4oSbY_7DOmv61BMVQ4hzXJ0a7oqNjRHLet28PkKEaLs_1uSSusypJdBvpTReWTDJl7HjrFxk2lvx7R_MB
