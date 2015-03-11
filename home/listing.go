package home

import (
	"fmt"
	"github.com/tdewolff/minify"
	"github.com/tdewolff/minify/html"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"time"
)

const (
	ListingCollectionName        = "Listings"
	ListingMarkeupCollectionName = "ListingsMarkup"
)

func NewDB(host, name string) *DB {
	return &DB{
		Host:        host,
		Name:        name,
		mongoBroker: newMongoBroker(host, name),
	}
}

type DB struct {
	Host        string
	Name        string
	mongoBroker *mongoBroker
}

func (self *DB) DoesListingExist(uri string) bool {

	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	query := collection.Find(bson.M{"listingurl": uri})

	// TODO -- is there a more efficient way to check existence?
	count, err := query.Count()

	if err != nil {
		// TODO - Should we be using panic?? Is it like exceptions?
		fmt.Println("Going to panic from err: ", err)
		panic(err)
	}

	if count > 0 {
		return true
	}
	return false
}

func (self *DB) UpdateListingStatus(listingId bson.ObjectId, forSale bool) error {
	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	err := collection.UpdateId(listingId, bson.M{
		"$set": bson.M{
			"isForSale":   forSale,
			"updatedDate": time.Now(),
		},
	})
	return err
}

func (self *DB) UpdateListingProperties(listingId bson.ObjectId, properties ListingProperties) error {
	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	err := collection.UpdateId(listingId, bson.M{
		"$set": bson.M{
			"isForSale":   true,
			"properties":  properties,
			"updatedDate": time.Now(),
		},
	})

	return err
}

func (self *DB) MarkupScraped(markupId bson.ObjectId) error {

	collection := self.mongoBroker.listingMarkupCollection()
	defer self.mongoBroker.closeCollection(collection)

	err := collection.UpdateId(markupId, bson.M{
		"$set": bson.M{
			"scrapedDate": time.Now(),
		},
	})

	return err
}

func (self *DB) GetNewestMarkupDate(listingId bson.ObjectId) (time.Time, bool) {
	collection := self.mongoBroker.listingMarkupCollection()
	defer self.mongoBroker.closeCollection(collection)

	query := collection.Find(bson.M{"listingId": listingId})
	query.Select(bson.M{"createdDate": 1})
	query.Sort("-createdDate")

	var result map[string]interface{}
	query.One(&result)

	var returnDate time.Time

	// If id wasn't returned, then we don't have a time
	_, found := result["_id"]

	if !found {
		return returnDate, false
	}

	returnDate, found = result["createdDate"].(time.Time)

	return returnDate, found
}

func (self *DB) SaveMarkup(listingId bson.ObjectId, uri, source, content string) error {
	collection := self.mongoBroker.listingMarkupCollection()
	defer self.mongoBroker.closeCollection(collection)

	// Get storable content string
	storableContent := prepareMarkupForStorage(content)

	doc := ListingMarkup{
		ListingId:   listingId,
		Url:         uri,
		Source:      source,
		Content:     storableContent,
		CreatedDate: time.Now(),
	}

	err := collection.Insert(doc)

	return err
}

func (self *DB) IterateListingsMarkup(limit int, handler func(ListingMarkup, *DB)) error {

	collection := self.mongoBroker.listingMarkupCollection()
	defer self.mongoBroker.closeCollection(collection)

	query := collection.Find(bson.M{"scrapedDate": bson.M{"$exists": false}})
	//query := collection.Find(nil)

	if limit != 0 {
		query.Limit(limit)
	}

	iter := query.Iter()

	var result ListingMarkup

	for iter.Next(&result) {
		handler(result, self)
	}

	if err := iter.Close(); err != nil {
		return err
	}
	return nil
}

func (self *DB) IterateAllListings(handler func(Listing, *DB)) error {

	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	iter := collection.Find(nil).Iter()

	var result Listing

	for iter.Next(&result) {
		handler(result, self)
	}

	if err := iter.Close(); err != nil {
		return err
	}
	return nil
}

// Model: Listing
type Listing struct {
	Id     bson.ObjectId `bson:"_id"`
	Url    string        `bson:"listingurl"`    // TODO - change this once fixed in db
	Source string        `bson:"listingsource"` // TODO - change this once fixed in db

	Properties ListingProperties `bson:"properties,omitempty"`

	// TODO - Later, change this to it's own model, under properties
	ImageUrls []string `bson:"imagelinks"` // TODO - change this once fixed in db

	ForSale     bool      `bson:"isForSale"`
	UpdatedDate time.Time `bson:"updatedDate,omitempty"`
	CheckDate   time.Time `bson:"checkDate,omitempty"`
}

// Nested Model: ListingProperties
type ListingProperties struct {
	CurrentPrice uint                   `bson:"currentPrice,omitempty"`
	MLS          string                 `bson:"mls"`
	Address      ListingAddress         `bson:"address,omitempty"`
	Location     GeoJson                `bson:"geoLocation,omitempty"`
	Meta         map[string]interface{} `bson:",inline"` // All extra data on this sub document.. aka super scheama
}

type ListingAddress struct {
	Street string `bson:"street"`
	City   string `bson:"city"`
	State  string `bson:"state"`
	Zip    string `bson:"zip"`
}

type GeoJson struct {
	Type        string    `bson:"type"`
	Coordinates []float64 `bson:"coordinates"`
}

// Model: ListingMarkup
type ListingMarkup struct {
	Id          bson.ObjectId `bson:"_id,omitempty"`
	ListingId   bson.ObjectId `bson:"listingId"`
	Url         string        `bson:"url"`
	Source      string        `bson:"source"`
	Content     string        `bson:"content"`
	CreatedDate time.Time     `bson:"createdDate"`
	ScrapedDate time.Time     `bson:"scrapedDate"`
}

// Cleanup the markup for storage
func prepareMarkupForStorage(rawMarkup string) string {

	m := minify.NewMinifier()
	m.Add("text/html", html.Minify)

	cleaned, err := m.MinifyString("text/html", rawMarkup)
	if err != nil {
		fmt.Println("Problem minifying markup: ", err)
		return rawMarkup
	}

	fmt.Printf("Compressed markup from: %v to: %v \n", len(rawMarkup), len(cleaned))

	return cleaned
}

// Mongo Broker
func newMongoBroker(host, name string) *mongoBroker {
	session, err := mgo.Dial(host)
	if err != nil {
		// TODO - Should we be using panic?? Is it like exceptions?
		fmt.Println("Going to panic from err: ", err)
		panic(err)
	}

	// Reads may not be entirely up-to-date, but they will always see the
	// history of changes moving forward, the data read will be consistent
	// across sequential queries in the same session, and modifications made
	// within the session will be observed in following queries (read-your-writes).
	// http://godoc.org/labix.org/v2/mgo#Session.SetMode
	session.SetMode(mgo.Monotonic, true)

	return &mongoBroker{
		Host:        host,
		DBName:      name,
		sessionPool: session,
	}
}

type mongoBroker struct {
	Host        string
	DBName      string
	sessionPool *mgo.Session
}

func (self *mongoBroker) collection(name string) *mgo.Collection {
	session := self.sessionPool.Copy()
	return session.DB(self.DBName).C(name)
}

func (self *mongoBroker) listingCollection() *mgo.Collection {
	return self.collection(ListingCollectionName)
}

func (self *mongoBroker) listingMarkupCollection() *mgo.Collection {
	return self.collection(ListingMarkeupCollectionName)
}

func (self *mongoBroker) closeCollection(collection *mgo.Collection) {
	collection.Database.Session.Close()
}
