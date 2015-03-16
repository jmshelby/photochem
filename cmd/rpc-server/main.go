package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	_ "strings"

	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/jmshelby/photochem/home"
)

var homeDb *home.DB

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

	// Start Up access to our listings
	homeDb = home.NewDB(dbHost, dbName)

	s := rpc.NewServer()
	// json-rpc version 2
	s.RegisterCodec(json2.NewCodec(), "application/json")
	s.RegisterCodec(json2.NewCodec(), "application/json; charset=UTF-8")
	s.RegisterService(new(WebService), "PhotoChem")

	// Wrap in my own handler for cors capability
	http.Handle("/rpc", &MyServer{s})
	http.ListenAndServe(":10000", nil)

}

// Web request stuff

type WebServiceListingRequest struct {
	Limit       uint     `json:"limit"`
	ExcludeIds  []string `json:"excludeIds"`
	IncludeIds  []string `json:"includeIds"`
	PriceMin    uint     `json:"minPrice"`
	PriceMax    uint     `json:"maxPrice"`
	Zip         string   `json:"zip"`
	ZipDistance uint     `json:"zip-meters"`
}

type WebServiceListingResponse struct {
	Listings      []WebServiceListing `json:"listings"`
	Total         int                 `json:"totalCount"`
	ResponseTotal int                 `json:"responseCount"`
	// return total listings available??
	// return current max??
	// return current count actually returned??
	// echo back filters???
}

type WebServiceListing struct {
	Id         string                   `json:"id"`
	Href       string                   `json:"href"`
	Properties interface{}              `json:"properties,omitempty"`
	Photos     []WebServiceListingPhoto `json:"photos"`
}

type WebServiceListingPhoto struct {
	Src string `json:"src"`
}

type WebService struct{}

func (self *WebService) GetListings(r *http.Request, args *WebServiceListingRequest, reply *WebServiceListingResponse) error {

	fmt.Printf("request: %+v\n", args)

	query := homeDb.NewListingsQuery()

	if args.Limit != 0 {
		query.LimitTo(args.Limit)
	}
	if len(args.ExcludeIds) > 0 {
		query.Exclude(args.ExcludeIds...)
	}
	if len(args.IncludeIds) > 0 {
		query.Include(args.IncludeIds...)
	}
	if args.PriceMin != 0 {
		query.PriceAbove(args.PriceMin)
	}
	if args.PriceMax != 0 {
		query.PriceUnder(args.PriceMax)
	}
	if args.Zip != "" && args.ZipDistance != 0 {
		query.NearZipCode(args.Zip, args.ZipDistance)
	}

	listings, total := query.Fetch()

	// TODO -- move the converting to listing structures to another function
	response := make([]WebServiceListing, len(*listings))
	for i, listing := range *listings {
		photos := make([]WebServiceListingPhoto, len(listing.ImageUrls))
		for photoIndex, image := range listing.ImageUrls {

			photos[photoIndex] = WebServiceListingPhoto{
				Src: image,
			}
		}

		response[i] = WebServiceListing{
			Id:         listing.Id.Hex(),
			Href:       listing.Url,
			Properties: listing.Properties,
			Photos:     photos,
		}
	}

	reply.Listings = response
	reply.Total = total
	reply.ResponseTotal = len(*listings)

	return nil
}

// TODO - Need call for error/alert notifications
// object type
// object id
// error code
// error message
// meta

// TODO - Need help call

type MyServer struct {
	r *rpc.Server
}

func (s *MyServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if origin := req.Header.Get("Origin"); origin != "" {
		rw.Header().Set("Access-Control-Allow-Origin", origin)
		rw.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		rw.Header().Set("Access-Control-Allow-Headers",
			"Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
	}
	// Stop here if its Preflighted OPTIONS request
	if req.Method == "OPTIONS" {
		return
	}
	// Lets Gorilla work
	s.r.ServeHTTP(rw, req)
}
