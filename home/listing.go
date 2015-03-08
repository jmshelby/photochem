package home

import (
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"time"
)

const (
	//ListingCollectionName        = "crawl_session.20150222121610" // TODO - change this once fixed in db
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

	// TODO -- do we want to do anything with the change info returned here??

	return err
}

// Model: Listing
type Listing struct {
	Id     bson.ObjectId `bson:"_id"`
	Url    string        `bson:"listingurl"`    // TODO - change this once fixed in db
	Source string        `bson:"listingsource"` // TODO - change this once fixed in db

	Properties ListingProperties `bson:"properties"`

	// TODO - Later, change this to it's own model
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
	Id          bson.ObjectId `bson:"_id"`
	ListingId   bson.ObjectId `bson:"listingId"`
	Url         string        `bson:"url"`
	Source      string        `bson:"source"`
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

// Mongo Broker
func newMongoBroker(host, name string) *mongoBroker {
	session, err := mgo.Dial(host)
	if err != nil {
		// TODO - Should we be using panic?? Is it like exceptions?
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
