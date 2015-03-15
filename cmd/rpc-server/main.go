package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func main() {
	s := rpc.NewServer()
	// json-rpc version 2
	s.RegisterCodec(CodecWithCors([]string{"*"}, json2.NewCodec()), "application/json")
	s.RegisterService(new(WebService), "PhotoChem")
	http.Handle("/rpc", s)
	http.ListenAndServe(":10000", nil)
}

// Web request stuff

type WebServiceListingRequest struct {
	Max        int      `json:"max"`
	ExcludeIds []string `json:"excludeIds"`
	PriceMin   int      `json:"minPrice"`
	PriceMax   int      `json:"maxPrice"`
	// zip code
	// lat/long with distance from
}

type WebServiceListingResponse struct {
	Listings []WebServiceListing `json:"listings"`
	Total    int                 `json:"totalCount"`
	// return total listings available??
	// return current max??
	// return current count actually returned??
	// echo back filters???
}

type WebServiceListing struct {
	Id     string                   `json:"id"`
	Href   string                   `json:"href"`
	Photos []WebServiceListingPhoto `json:"photos"`
}

type WebServiceListingPhoto struct {
	Tag string `json:"tag"`
	Src string `json:"src"`
}

type WebService struct{}

func (self *WebService) GetRawListings(r *http.Request, args *WebServiceListingRequest, reply *[]HouseListing) error {
	listings, _ := getListings(999, nil)
	*reply = listings
	return nil
}

func (self *WebService) GetListings(r *http.Request, args *WebServiceListingRequest, reply *WebServiceListingResponse) error {

	fmt.Printf("request: %+v\n", args)

	listings, total := getListings(args.Max, args.ExcludeIds)

	// TODO -- move the converting to listing structures to another function
	response := make([]WebServiceListing, len(listings))
	for i, listing := range listings {
		photos := make([]WebServiceListingPhoto, len(listing.Images))
		for photoIndex, image := range listing.Images {

			photos[photoIndex] = WebServiceListingPhoto{
				Tag: "unknown",
				Src: image,
			}
		}

		response[i] = WebServiceListing{
			Id:     listing.Id.Hex(),
			Href:   listing.ListingUrl,
			Photos: photos,
		}
	}

	reply.Listings = response
	reply.Total = total

	return nil
}

// TODO - Need call for error/alert notifications
// object type
// object id
// error code
// error message
// meta

// TODO - Need help call

// mongo shit

var listingsCollection *mgo.Collection

func collection() *mgo.Collection {
	if listingsCollection == nil {
		mongoSession, _ := mgo.Dial("hqopti1")
		listingsCollection = mongoSession.DB("PhotoChemistry-Live").C("Listings")
	}
	return listingsCollection
}

type HouseListing struct {
	Id            bson.ObjectId `bson:"_id"`
	ListingUrl    string        `bson:"listingurl"`
	ListingSource string        `bson:"listingsource"`
	Images        []string      `bson:"imagelinks"`
}

func getListings(limit int, skip []string) ([]HouseListing, int) {

	count, _ := collection().Count()
	fmt.Println("Count from remote mongodb: ", count)

	objectIds := make([]bson.ObjectId, len(skip))
	for i, idString := range skip {
		objectIds[i] = bson.ObjectIdHex(idString)
	}

	query := collection().Find(bson.M{
		"_id": bson.M{
			"$nin": objectIds,
		},
	})

	query.Limit(limit)

	var result []HouseListing

	query.All(&result)

	return result, count
}

func CodecWithCors(corsDomains []string, unpimped rpc.Codec) rpc.Codec {
	return corsCodec{corsDomains, unpimped}
}

type corsCodecRequest struct {
	corsDomains            []string
	underlyingCodecRequest rpc.CodecRequest
}

//override exactly one method of the underlying anonymous field and delegate to it.
func (ccr corsCodecRequest) WriteResponse(w http.ResponseWriter, reply interface{}) {
	w.Header().Add("Access-Control-Allow-Origin", strings.Join(ccr.corsDomains, " "))
	ccr.underlyingCodecRequest.WriteResponse(w, reply)
}

func (ccr corsCodecRequest) Method() (string, error) {
	return ccr.underlyingCodecRequest.Method()
}

func (ccr corsCodecRequest) ReadRequest(request interface{}) error {
	return ccr.underlyingCodecRequest.ReadRequest(request)
}

func (ccr corsCodecRequest) WriteError(w http.ResponseWriter, status int, err error) {
	ccr.underlyingCodecRequest.WriteError(w, status, err)
}

type corsCodec struct {
	corsDomains     []string
	underlyingCodec rpc.Codec
}

//override exactly one method of the underlying anonymous field and delegate to it.
func (cc corsCodec) NewRequest(req *http.Request) rpc.CodecRequest {
	return corsCodecRequest{cc.corsDomains, cc.underlyingCodec.NewRequest(req)}
}
