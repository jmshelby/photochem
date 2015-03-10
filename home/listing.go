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

func (self *DB) UpsertListing(uri string, listing Listing) error {
	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	_, err := collection.Upsert(bson.M{"listingurl": uri}, listing)

	// TODO -- do we want to do anything with the change info returned here above??

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

	// TODO -- need to add query to filter unscraped markup
	query := collection.Find(nil)

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

	Properties ListingProperties `bson:"properties"`

	// TODO - Later, change this to it's own model, under properties
	ImageUrls []string `bson:"imagelinks"` // TODO - change this once fixed in db

	Status      ListingStatus `bson:"listingStatus"`
	UpdatedDate time.Time     `bson:"updatedDate,omitempty"`
	CheckDate   time.Time     `bson:"checkDate,omitempty"`
}

// Nested Model: ListingProperties
type ListingProperties struct {
	CurrentPrice int                    `bson:"currentPrice,omitempty"`
	Address      ListingAddress         `bson:"address,omitempty"`
	Meta         map[string]interface{} `bson:",inline"` // All extra data on this sub document.. aka super scheama
}

type ListingAddress struct {
	Street1 string `json:"street1"`
	Street2 string `bson:"street2"`
	City    string `bson:"city"`
	State   string `bson:"state"`
	Zip     string `bson:"zip"`
}

// Model: ListingMarkup
type ListingMarkup struct {
	Id          bson.ObjectId `bson:"_id,omitempty"`
	ListingId   bson.ObjectId `bson:"listingId"`
	Url         string        `bson:"url"`
	Source      string        `bson:"source"`
	Content     string        `bson:"content"`
	CreatedDate time.Time     `bson:"createdDate"`
}

// Status Enum
type ListingStatus int

const (
	ForSale ListingStatus = 1 + iota
	OffMarket
)

var listingStatuses = [...]string{
	"for_sale",
	"off_market",
}

func (status ListingStatus) String() string { return listingStatuses[status-1] }

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
