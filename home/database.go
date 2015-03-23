package home

import (
	"errors"
	"fmt"
	"github.com/nf/geocode"
	//"github.com/tdewolff/minify"
	//"github.com/tdewolff/minify/html"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	_ "strconv"
	"time"
)

const (
	ListingCollectionName        = "Listings"
	ListingMarkeupCollectionName = "ListingsMarkup"
)

func NewDB(host, name string) *DB {
	newDb := &DB{
		Host:        host,
		Name:        name,
		mongoBroker: newMongoBroker(host, name),
	}

	// Make sure we have the proper indexes defined
	newDb.ensureIndexes()

	return newDb
}

type DB struct {
	Host        string
	Name        string
	mongoBroker *mongoBroker
}

func (self *DB) ensureIndexes() {
	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)
	collection.EnsureIndex(mgo.Index{Key: []string{"$2dsphere:properties.geoLocation"}})
	collection.EnsureIndex(mgo.Index{Key: []string{"listingUrl"}})
	collection.EnsureIndex(mgo.Index{Key: []string{"isForSale"}})
	collection.EnsureIndex(mgo.Index{Key: []string{"properties.address.state"}})
	collection.EnsureIndex(mgo.Index{Key: []string{"properties.address.city"}})
}

func (self *DB) GetListingIdFromUrl(uri string) (bson.ObjectId, error) {
	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	type document struct {
		Id bson.ObjectId `bson:"_id"`
	}
	listing := document{}

	err := collection.Find(bson.M{"listingUrl": uri}).Select(bson.M{"_id": 1}).One(&listing)

	if err != nil {
		// Not found
		return listing.Id, errors.New("Listing not found")
	}

	return listing.Id, nil
}

func (self *DB) RegisterListing(uri, source, markup string) (Listing, bool, error) {

	listing := Listing{}

	// Create scraper
	scraper, err := NewScraper(markup)
	if err != nil {
		// Can't do anything if the markup is unparsable
		return listing, false, err
	}

	// Make sure it's for sale..
	if !scraper.IsForSale() {
		// Update status by uri (in case it's already registered)
		self.UpdateListingStatusByUrl(uri, false)
		return listing, false, errors.New("Listing is not on the market")
	}

	// Pull out Images
	images := scraper.ScrapeListingImages()
	if len(images) == 0 {
		// Update status by uri (in case it's already registered)
		self.UpdateListingStatusByUrl(uri, false)
		return listing, false, errors.New("Listing is not useful, no house images found")
	}

	// Pull out properties
	properties := scraper.ScrapeListingProperties()
	// TODO - find a way to validate these, to update status

	// Create Listing object
	listing = Listing{
		Url:         uri,
		Source:      source,
		ForSale:     true,
		UpdatedDate: time.Now(),
		Images:      images,
		Properties:  properties,
	}

	// Save Listing
	listingId, insertedFl, err := self.SaveListing(listing)
	if err != nil {
		// Problem when saving .... the caller might want to try again??
		return listing, false, err
	}

	// Add the ID
	listing.Id = listingId

	// return new listing
	return listing, !insertedFl, nil
}

// SaveListing will perform an upsert, based on the listing Url.
// If a listing was created/inserted, the second return value
// will be true, other-wise false. Error will be returned if
// the listing was not able to be saved. The first return value
// is the current ID for the record.
func (self *DB) SaveListing(listing Listing) (bson.ObjectId, bool, error) {
	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	changeInfo, err := collection.Upsert(bson.M{"listingUrl": listing.Url}, listing)
	if err != nil {
		var nothing bson.ObjectId
		return nothing, false, err
	}

	// Check if id was returned from the upsert call (if it was inserted)
	listingId, cast := changeInfo.UpsertedId.(bson.ObjectId)
	if !cast {
		// Get the id from the url
		listingId, idErr := self.GetListingIdFromUrl(listing.Url)
		if idErr != nil {
			// This should never happen
			return listingId, false, idErr
		}
	}

	if changeInfo.Updated != 0 {
		// It was updated, return false
		return listingId, false, nil
	} else {
		// It was created, return true
		return listingId, true, nil
	}

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

func (self *DB) UpdateListingStatusByUrl(listingUrl string, forSale bool) error {
	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	err := collection.Update(
		bson.M{
			"listingUrl": listingUrl,
		},
		bson.M{
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

	// TODO - cleanup, delete the other previous markup documents

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

func (self *DB) IterateListingsOlderThan(staleDate time.Time, limit int, handler func(Listing, *DB)) error {

	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	query := collection.Find(bson.M{"updatedDate": bson.M{"$lt": staleDate}})

	if limit != 0 {
		query.Limit(limit)
	}

	iter := query.Iter()

	var result Listing

	for iter.Next(&result) {
		handler(result, self)
	}

	if err := iter.Close(); err != nil {
		return err
	}
	return nil

}

func (self *DB) QueryListings(query ListingsQuery) (*[]Listing, int) {

	collection := self.mongoBroker.listingCollection()
	defer self.mongoBroker.closeCollection(collection)

	q := collection.Find(query.buildMongoQuery())

	// Get the total count from the query
	count, _ := q.Count()

	if query.limitFl {
		q.Limit(int(query.limit))
	}

	var result []Listing

	q.All(&result)

	return &result, count
}

func (self *DB) NewListingsQuery() *ListingsQuery {
	query := ListingsQuery{db: self}
	query.Init()
	return &query
}

type ListingsQuery struct {
	limitFl bool
	limit   uint

	excludeFl bool
	excluding []string
	includeFl bool
	including []string

	priceMinFl bool
	priceMin   uint
	priceMaxFl bool
	priceMax   uint

	locationFl          bool
	locationZip         string
	locationZipDistance uint
	location            GeoJson

	db *DB
}

func (self *ListingsQuery) Fetch() (*[]Listing, int) {
	return self.db.QueryListings(*self)
}

func (self *ListingsQuery) Init() {
	self.limitFl = false
	self.excludeFl = false
	self.excluding = []string{}
	self.includeFl = false
	self.including = []string{}
	self.priceMinFl = false
	self.priceMaxFl = false
	self.locationFl = false
}

func (self *ListingsQuery) LimitTo(limit uint) {
	self.limit = limit
	self.limitFl = true
}

func (self *ListingsQuery) Exclude(ids ...string) {
	self.excluding = append(self.excluding, ids...)
	self.excludeFl = true
}

func (self *ListingsQuery) Include(ids ...string) {
	self.including = append(self.including, ids...)
	self.includeFl = true
}

func (self *ListingsQuery) PriceAbove(filter uint) {
	self.priceMin = filter
	self.priceMinFl = true
}

func (self *ListingsQuery) PriceUnder(filter uint) {
	self.priceMax = filter
	self.priceMaxFl = true
}

func (self *ListingsQuery) PriceBetween(min uint, max uint) {
	self.PriceAbove(min)
	self.PriceUnder(max)
}

// TODO - Add error handling here later
func (self *ListingsQuery) NearZipCode(zip string, distance uint) {
	self.locationFl = true
	// Just set the recevied data so we have it
	self.locationZip = zip
	self.locationZipDistance = distance
	// Get the actual geo json coords
	self.location = FetchZipCodeCoords(zip)
}

func (self *ListingsQuery) buildMongoQuery() bson.M {
	query := bson.M{}

	idQuery := bson.M{}
	if self.excludeFl {
		idQuery["$nin"] = objectIds(self.excluding)
	}
	if self.includeFl {
		idQuery["$in"] = objectIds(self.including)
	}
	if self.includeFl || self.excludeFl {
		query["_id"] = idQuery
	}

	priceQuery := bson.M{}
	if self.priceMaxFl {
		priceQuery["$lt"] = self.priceMax
	}
	if self.priceMinFl {
		priceQuery["$gt"] = self.priceMin
	}
	if self.priceMaxFl || self.priceMinFl {
		query["properties.currentPrice"] = priceQuery
	}

	if self.locationFl {
		query["properties.geoLocation"] = bson.M{
			"$nearSphere": bson.M{
				"$geometry":    self.location,
				"$minDistance": 0,
				"$maxDistance": self.locationZipDistance,
			},
		}
	}
	fmt.Printf("mongo query: %+v\n", query)

	return query
}

// Cleanup the markup for storage
func prepareMarkupForStorage(rawMarkup string) string {

	/*
		m := minify.NewMinifier()
		m.Add("text/html", html.Minify)

		cleaned, err := m.MinifyString("text/html", rawMarkup)
		if err != nil {
			fmt.Println("Problem minifying markup: ", err)
			return rawMarkup
		}

		fmt.Printf("Compressed markup from: %v to: %v \n", len(rawMarkup), len(cleaned))

		return cleaned
	*/
	return rawMarkup
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

// TODO - add error handling here later
var zipCache map[string]GeoJson = make(map[string]GeoJson)

func FetchZipCodeCoords(zip string) GeoJson {

	cache, cacheFound := zipCache[zip]
	if cacheFound {
		fmt.Println("returning cached zip data")
		return cache
	}

	req := &geocode.Request{Provider: geocode.GOOGLE, Region: "us", Address: zip}

	resp, err := req.Lookup(nil)

	if err != nil {
		fmt.Println("Error when fetching zip coords: ", err)
		return GeoJson{Type: "Point"}
	}

	if resp.Status != "OK" {
		fmt.Println("Bad response when fetching zip coords: ", resp)
		return GeoJson{Type: "Point"}
	}

	fmt.Printf("google resp: %+v\n", resp)

	lat := resp.GoogleResponse.Results[0].Geometry.Location.Lat
	lng := resp.GoogleResponse.Results[0].Geometry.Location.Lng

	returnCoords := GeoJson{
		Type:        "Point",
		Coordinates: []float64{lng, lat},
	}

	// Cache it
	zipCache[zip] = returnCoords

	return returnCoords
}

func objectIds(idStrings []string) []bson.ObjectId {
	objectIds := make([]bson.ObjectId, len(idStrings))
	for i, idString := range idStrings {
		objectIds[i] = bson.ObjectIdHex(idString)
	}
	return objectIds
}
