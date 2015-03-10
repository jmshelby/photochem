package main

import (
	//"bytes"
	//"crypto/tls"
	"flag"
	"fmt"
	"strings"
	//"io"
	//"net/http"
	"os"
	"strconv"
	//"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jmshelby/photochem/home"
)

const (
	DefaultNumberOfWorkers = 3
)

type FieldSelector struct {
	FieldName   string
	Selector    string
	Attribute   string // If you just want the text of the element(s), leave blank
	MultiValued bool
}

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

	fmt.Println("Number of workers: ", numberOfWorkers)

	// Start Up access to our listings
	var homeDb *home.DB
	homeDb = home.NewDB(dbHost, dbName)

	fmt.Println("We have our home database, now we're gonna iterate..")

	// Make channel, buffered with expected number of workers
	//listingQueue := make(chan home.Listing, numberOfWorkers)

	// Start up workers
	//for i := 0; i < numberOfWorkers; i++ {
	//fmt.Println("Starting up worker ", i+1)
	//go queueWorker(listingQueue, homeDb)
	//}

	fieldSelectors := []FieldSelector{

		// {"street", "[itemprop=streetAddress]", "", false},
		// {"city", "[itemprop=addressLocality]", "", false},
		// {"state", "[itemprop=addressRegion]", "", false},
		// {"zip", "[itemprop=postalCode]", "", false},

		{"propertyId", "[name=propid]", "value", false},
		{"supplierId", "[name=supplierid]", "value", false},
		{"agentId", "[name=agentid]", "value", false},
		{"mls", "[name=MLSNumber]", "value", false},
		{"price", "[name=Price]", "value", false},
		{"street", "[name=Address]", "value", false},
		{"state", "[name=State]", "value", false},
		{"city", "[name=City]", "value", false},
		{"zip", "[name=Zip]", "value", false},

		{"latitude", "[itemprop=latitude]", "content", false},
		{"longitude", "[itemprop=longitude]", "content", false},
	}

	//<input type="hidden" value="226137560" name="propid" id="propid" />
	//<input type="hidden" name="supplierid" id="supplierid" value="43" />
	//<input type="hidden" name="agentid" id="agentid" value="404492" />
	//<input type="hidden" name="MLSNumber" value="1506331" />
	//<input type="hidden" name="Address" id="Address" value="305 Brooke Ave" />
	//<input type="hidden" name="State" id="State" value="VA" />
	//<input type="hidden" name="City" id="City" value="Norfolk" />
	//<input type="hidden" name="Price" id="Price" value="699900" />
	//<input type="hidden" name="Zip" id="Zip" value="23510" />

	//<meta itemprop="latitude" content="36.849484" />
	//<meta itemprop="longitude" content="-76.295258" />

	i := 0
	homeDb.IterateListingsMarkup(1000, func(markup home.ListingMarkup, db *home.DB) {
		i++

		doc := makeDocument(markup.Content)

		status, _ := doc.Find("input[name=listing_status]").First().Attr("value")
		fmt.Printf("%v) [%v] %v\n", i, status, markup.Url)

		if status != "FOR SALE" {
			fmt.Printf("--> No longer for sale\n")
			return
		}

		//var scrapedFields = make(map[string]string)
		for _, v := range fieldSelectors {

			value := getFieldValueFromSelector(doc, v)

			//scrapedFields[k] = value

			fmt.Printf("-> %v: %v\n", v.FieldName, value)
		}

	})
	fmt.Println("Looped on this many: ", i)

	// Close Queue
	//close(listingQueue)

	fmt.Println("Somehow we're done with all of our listings... exiting.")
}

func getFieldValueFromSelector(doc *goquery.Document, selector FieldSelector) string {

	var returnValue string

	if selector.Attribute != "" {
		// TODO -- add error handling.... somehow...
		returnValue, _ = doc.Find(selector.Selector).First().Attr(selector.Attribute)
	} else {
		returnValue = doc.Find(selector.Selector).First().Text()
	}

	// TODO - add support for multi values somehow...

	return returnValue
}

func makeDocument(markup string) *goquery.Document {

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(markup))
	if err != nil {
		fmt.Printf("[ERR] Couldn't parse page: %s\n", err)
		return nil
	}

	return doc
}
