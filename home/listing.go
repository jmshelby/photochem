package home

import (
	"gopkg.in/mgo.v2/bson"
	"time"
)

// Listing Model
type Listing struct {
	Id     bson.ObjectId `bson:"_id,omitempty"`
	Url    string        `bson:"listingUrl"`
	Source string        `bson:"listingSource"`

	Properties ListingProperties `bson:"properties,omitempty"`

	Images []ListingImage `bson:"images"`

	ForSale     bool      `bson:"isForSale"`
	UpdatedDate time.Time `bson:"updatedDate,omitempty"`
}

// Listing Model - Images
type ListingImage struct {
	Url   string
	Label string
	Tags  []string
}

// Listing Model - Properties
type ListingProperties struct {
	CurrentPrice uint                   `bson:"currentPrice,omitempty"`
	MLS          string                 `bson:"mls"`
	Address      ListingAddress         `bson:"address,omitempty"`
	Location     GeoJson                `bson:"geoLocation,omitempty"`
	Meta         map[string]interface{} `bson:",inline" json:"-"` // All extra data on this sub document.. aka super scheama
}

// Listing Model - Properties / Address
type ListingAddress struct {
	Street string `bson:"street"`
	City   string `bson:"city"`
	State  string `bson:"state"`
	Zip    string `bson:"zip"`
}

// Listing Model - Properties / Location
type GeoJson struct {
	Type        string    `bson:"type"`
	Coordinates []float64 `bson:"coordinates"`
}

// ListingMarkup Model
type ListingMarkup struct {
	Id          bson.ObjectId `bson:"_id,omitempty"`
	ListingId   bson.ObjectId `bson:"listingId"`
	Url         string        `bson:"url"`
	Source      string        `bson:"source"`
	Content     string        `bson:"content"`
	CreatedDate time.Time     `bson:"createdDate"`
	ScrapedDate time.Time     `bson:"scrapedDate"`
}
