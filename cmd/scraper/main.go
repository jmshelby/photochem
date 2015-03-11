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

	i := 0
	offMarket := 0
	homeDb.IterateListingsMarkup(200000, func(markup home.ListingMarkup, db *home.DB) {
		i++

		doc := makeDocument(markup.Content)

		status, _ := doc.Find("form.nav-search input[name=listing_status]").First().Attr("value")
		fmt.Printf("%v) [%v] - %v - %v\n", i, status, markup.ListingId, markup.Url)

		if status != "FOR SALE" {
			fmt.Printf("--> No longer for sale\n")
			db.UpdateListingStatus(markup.ListingId, false)
			db.MarkupScraped(markup.Id)
			offMarket++
			return
		}

		var out = make(map[string]string)
		for _, v := range fieldSelectors {

			value := getFieldValueFromSelector(doc, v)

			out[v.FieldName] = value

			fmt.Printf("  -> %v: %v\n", v.FieldName, value)
		}

		//fmt.Printf("  => Scrapped Fields: %+v\n", out)

		lat, _ := strconv.ParseFloat(out["latitude"], 64)
		long, _ := strconv.ParseFloat(out["longitude"], 64)
		price, _ := strconv.Atoi(out["price"])

		// Build listing properties structure
		props := home.ListingProperties{
			CurrentPrice: uint(price),
			MLS:          out["mls"],
			Location: home.GeoJson{
				Type:        "Point",
				Coordinates: []float64{long, lat},
			},
			Address: home.ListingAddress{
				Street: out["street"],
				City:   out["city"],
				State:  out["state"],
				Zip:    out["zip"],
			},
		}

		props.Meta = map[string]interface{}{
			"propertyId": out["propertyId"],
			"supplierId": out["supplierId"],
			"agentId":    out["agentId"],
		}

		db.UpdateListingProperties(markup.ListingId, props)
		db.MarkupScraped(markup.Id)

	})
	fmt.Println("Looped on this many: ", i)
	fmt.Println("Off Market count: ", offMarket)

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
