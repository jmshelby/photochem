package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

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
	s.RegisterCodec(CodecWithCors([]string{"*"}, json2.NewCodec()), "application/json")
	s.RegisterCodec(CodecWithCors([]string{"*"}, json2.NewCodec()), "application/json; charset=UTF-8")
	s.RegisterService(new(WebService), "PhotoChem")
	http.Handle("/rpc", s)
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

// mongo shit

func CodecWithCors(corsDomains []string, unpimped rpc.Codec) rpc.Codec {
	return corsCodec{corsDomains, unpimped}
}

type corsCodecRequest struct {
	corsDomains []string
	rpc.CodecRequest
}

//override exactly one method of the underlying anonymous field and delegate to it.
func (ccr corsCodecRequest) WriteResponse(w http.ResponseWriter, reply interface{}) {
	w.Header().Add("Access-Control-Allow-Origin", strings.Join(ccr.corsDomains, " "))
	ccr.CodecRequest.WriteResponse(w, reply)
}

type corsCodec struct {
	corsDomains []string
	rpc.Codec
}

//override exactly one method of the underlying anonymous field and delegate to it.
func (cc corsCodec) NewRequest(req *http.Request) rpc.CodecRequest {
	return corsCodecRequest{cc.corsDomains, cc.Codec.NewRequest(req)}
}
