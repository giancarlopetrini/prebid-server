package sonobi

import (
	"encoding/json"
	"fmt"
	"github.com/mxmCherry/openrtb"
	"github.com/prebid/prebid-server/adapters"
	"github.com/prebid/prebid-server/errortypes"
	"github.com/prebid/prebid-server/openrtb_ext"
	"net/http"
)

// SonobiAdapter - Sonobi SonobiAdapter definition
type SonobiAdapter struct {
	http *adapters.HTTPAdapter
	URI  string
}

// Name returns the name fo cookie stuff
func (a *SonobiAdapter) Name() string {
	return "sonobi"
}

//SkipNoCookies flag for skipping no cookies...
func (a *SonobiAdapter) SkipNoCookies() bool {
	return false
}

// NewSonobiAdapter create a new SovrnSonobiAdapter instance
func NewSonobiAdapter(config *adapters.HTTPAdapterConfig, endpoint string) *SonobiAdapter {
	return NewSonobiBidder(adapters.NewHTTPAdapter(config).Client, endpoint)
}

// NewSonobiBidder Initializes the Bidder
func NewSonobiBidder(client *http.Client, endpoint string) *SonobiAdapter {
	a := &adapters.HTTPAdapter{Client: client}

	return &SonobiAdapter{
		http: a,
		URI:  endpoint,
	}
}

type sonobiParams struct {
	TagID string `json:"TagID"`
}

// MakeRequests Makes the OpenRTB request payload
func (a *SonobiAdapter) MakeRequests(request *openrtb.BidRequest) ([]*adapters.RequestData, []error) {
	var errs []error
	var sonobiExt openrtb_ext.ExtImpSonobi
	var err error

	var adapterRequests []*adapters.RequestData

	// Sonobi currently only supports 1 imp per request to sonobi.
	// Loop over the imps from the initial bid request to form many adapter requests to sonobi with only 1 imp.
	for _, imp := range request.Imp {
		// Make a copy as we don't want to change the original request
		reqCopy := *request
		reqCopy.Imp = append(make([]openrtb.Imp, 0, 1), imp)

		var bidderExt adapters.ExtImpBidder
		if err = json.Unmarshal(reqCopy.Imp[0].Ext, &bidderExt); err != nil {
			errs = append(errs, err)
			continue
		}

		if err = json.Unmarshal(bidderExt.Bidder, &sonobiExt); err != nil {
			errs = append(errs, err)
			continue
		}

		reqCopy.Imp[0].TagID = sonobiExt.TagID

		adapterReq, errors := a.makeRequest(&reqCopy)
		if adapterReq != nil {
			adapterRequests = append(adapterRequests, adapterReq)
		}
		errs = append(errs, errors...)
	}

	return adapterRequests, errs

}

// makeRequest helper method to crete the http request data
func (a *SonobiAdapter) makeRequest(request *openrtb.BidRequest) (*adapters.RequestData, []error) {

	var errs []error

	reqJSON, err := json.Marshal(request)

	if err != nil {
		errs = append(errs, err)
		return nil, errs
	}

	headers := http.Header{}
	headers.Add("Content-Type", "application/json;charset=utf-8")
	headers.Add("Accept", "application/json")
	return &adapters.RequestData{
		Method:  "POST",
		Uri:     a.URI,
		Body:    reqJSON,
		Headers: headers,
	}, errs
}

// MakeBids makes the bids
func (a *SonobiAdapter) MakeBids(internalRequest *openrtb.BidRequest, externalRequest *adapters.RequestData, response *adapters.ResponseData) (*adapters.BidderResponse, []error) {
	var errs []error

	if response.StatusCode == http.StatusNoContent {
		return nil, nil
	}

	if response.StatusCode == http.StatusBadRequest {
		return nil, []error{&errortypes.BadInput{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info", response.StatusCode),
		}}
	}

	if response.StatusCode != http.StatusOK {
		return nil, []error{&errortypes.BadServerResponse{
			Message: fmt.Sprintf("Unexpected status code: %d. Run with request.debug = 1 for more info", response.StatusCode),
		}}
	}

	var bidResp openrtb.BidResponse

	if err := json.Unmarshal(response.Body, &bidResp); err != nil {
		return nil, []error{err}
	}

	bidResponse := adapters.NewBidderResponseWithBidsCapacity(5)

	for _, sb := range bidResp.SeatBid {
		for i := range sb.Bid {
			bidType, err := getMediaTypeForImp(sb.Bid[i].ImpID, internalRequest.Imp)
			if err != nil {
				errs = append(errs, err)
			} else {
				b := &adapters.TypedBid{
					Bid:     &sb.Bid[i],
					BidType: bidType,
				}
				bidResponse.Bids = append(bidResponse.Bids, b)
			}
		}
	}
	return bidResponse, errs
}

func getMediaTypeForImp(impID string, imps []openrtb.Imp) (openrtb_ext.BidType, error) {
	mediaType := openrtb_ext.BidTypeBanner
	for _, imp := range imps {
		if imp.ID == impID {
			if imp.Banner == nil && imp.Video != nil {
				mediaType = openrtb_ext.BidTypeVideo
			}
			return mediaType, nil
		}
	}

	// This shouldnt happen. Lets handle it just incase by returning an error.
	return "", &errortypes.BadInput{
		Message: fmt.Sprintf("Failed to find impression \"%s\" ", impID),
	}
}

func addHeaderIfNonEmpty(headers http.Header, headerName string, headerValue string) {
	if len(headerValue) > 0 {
		headers.Add(headerName, headerValue)
	}
}