package seattlefoodtruck

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
)

//NeighborhoodsAPIService neighborhoods api
type NeighborhoodsAPIService service

//GetNeighborhoods returns neighborhoods in seattle where you can find foodtrucks
func (n *NeighborhoodsAPIService) GetNeighborhoods(ctx context.Context) ([]Neighborhood, *http.Response, error) {
	var (
		httpMethod  = strings.ToUpper("get")
		returnValue []Neighborhood
		payload     interface{}
	)
	endpoint := fmt.Sprintf("%s/%s", n.client.cfg.BasePath, "/neighborhoods")
	log.Infof("Endpoint: %s", endpoint)

	queryParams := url.Values{}
	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "application/json"

	r, err := n.client.prepareRequest(ctx, endpoint, httpMethod, payload, headers, queryParams)
	if err != nil {
		return returnValue, nil, err
	}
	httpResponse, err := n.client.callAPI(r)
	if err != nil || httpResponse == nil {
		return returnValue, httpResponse, err
	}
	defer httpResponse.Body.Close()
	responsePayload, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return returnValue, httpResponse, err
	}
	if httpResponse.StatusCode == 200 {
		err = n.client.decode(&returnValue, responsePayload, httpResponse.Header.Get("Content-Type"))
		if err != nil {
			return returnValue, httpResponse, err
		}
	}
	return returnValue, httpResponse, nil
}
