package home

import (
	//"fmt"
	"github.com/PuerkitoBio/goquery"
	"strconv"
	"strings"
)

type MarkupFieldSelector struct {
	FieldName   string
	Selector    string
	Attribute   string // If you just want the text of the element(s), leave blank
	MultiValued bool
}

// Default Field Selectors
var fieldSelectors []MarkupFieldSelector = []MarkupFieldSelector{

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

	// {"street", "[itemprop=streetAddress]", "", false},
	// {"city", "[itemprop=addressLocality]", "", false},
	// {"state", "[itemprop=addressRegion]", "", false},
	// {"zip", "[itemprop=postalCode]", "", false},
}

func GetDefaultFieldSelectors() []MarkupFieldSelector {
	return fieldSelectors
}

func NewScraper(markup string) (*Scraper, error) {

	// Parse into document first
	document, err := MakeDocumentFromMarkup(markup)
	if err != nil {
		return nil, err
	}

	newScraper := Scraper{
		Markup:         markup,
		FieldSelectors: GetDefaultFieldSelectors(),
		doc:            document,
	}
	return &newScraper, nil
}

func MakeDocumentFromMarkup(markup string) (*goquery.Document, error) {

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(markup))

	return doc, err
}

func GetFieldValueFromSelector(doc *goquery.Document, selector MarkupFieldSelector) string {

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

type Scraper struct {
	Markup         string
	FieldSelectors []MarkupFieldSelector
	doc            *goquery.Document
}

func (self *Scraper) IsForSale() bool {

	// Take a look at the listing_status input in the search nav secontion
	status, _ := self.doc.Find("form.nav-search input[name=listing_status]").First().Attr("value")

	// Should be "FOR SALE" string
	// TODO - can we rely on this??
	return (status == "FOR SALE")

}

func (self *Scraper) ScrapeField(selector MarkupFieldSelector) string {
	return GetFieldValueFromSelector(self.doc, selector)
}

func (self *Scraper) ScrapeFields() map[string]string {

	// TODO -- see if we need this
	//if len(self.FieldSelectors) == 0 {
	//return make(map[string]string)
	//}

	values := make(map[string]string)
	for _, fieldSelector := range self.FieldSelectors {

		values[fieldSelector.FieldName] = self.ScrapeField(fieldSelector)

	}

	return values
}

func (self *Scraper) ScrapeListingImages() []ListingImage {

	images := make([]ListingImage, 0)
	// TODO - base this off of an injectable selector
	self.doc.Find("#slider img").Each(func(i int, s *goquery.Selection) {

		imageLink, _ := s.Attr("src")

		if imageLink != "" {
			images = append(images, ListingImage{Url: imageLink})
		}
	})

	return images
}

// ScrapeListingProperties will scrape all configured fields, and then marshal/convert
// the raw string types into the types required by each of the listing properties. Extra
// scraped data will be thrown into the meta properties.
func (self *Scraper) ScrapeListingProperties() ListingProperties {

	raw := self.ScrapeFields()

	lat, _ := strconv.ParseFloat(raw["latitude"], 64)
	long, _ := strconv.ParseFloat(raw["longitude"], 64)
	price, _ := strconv.Atoi(raw["price"])

	// Build listing properties structure
	props := ListingProperties{
		CurrentPrice: uint(price),
		MLS:          raw["mls"],
		Location: GeoJson{
			Type:        "Point",
			Coordinates: []float64{long, lat},
		},
		Address: ListingAddress{
			Street: raw["street"],
			City:   raw["city"],
			State:  raw["state"],
			Zip:    raw["zip"],
		},
	}

	// TODO - do this in a better way with introspection
	delete(raw, "latitude")
	delete(raw, "longitude")
	delete(raw, "price")
	delete(raw, "mls")
	delete(raw, "street")
	delete(raw, "city")
	delete(raw, "state")
	delete(raw, "zip")

	// Add extra non-standard fields to meta, as raw
	meta := make(map[string]interface{})
	for k, v := range raw {
		meta[k] = v
	}
	props.Meta = meta

	return props
}
