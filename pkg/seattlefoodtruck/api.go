package seattlefoodtruck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	ws "github.com/appsbyram/pkg/http"
	l "github.com/appsbyram/pkg/logging"
	"go.uber.org/zap"
)

const (
	//Today for today
	Today = "today"

	//Tomorrow for tomorrow
	Tomorrow = "tomorrow"

	//EventsResourcePath represents path to retrieve a collection of event resources
	EventsResourcePath = "events"

	//LocationResourcePath represents path to retrieve a location resource
	LocationResourcePath = "locations/%s"
)

//FoodTruckClient represents generic interface for Seattle FoodTruck API client
type FoodTruckClient interface {
	GetEvents(id string, onDay string) ([]Event, error)
	GetLocation(id string) (Location, error)
}

type foodTruckClient struct {
	host     string
	scheme   string
	basePath string

	client *http.Client
	logger *zap.SugaredLogger
}

//NewFoodTruckClient returns a new instance of Food Truck Client
func NewFoodTruckClient(ctx context.Context, host, scheme, basePath string) FoodTruckClient {
	logger := l.LoggerFromContext(ctx)

	return &foodTruckClient{
		host:     host,
		scheme:   scheme,
		basePath: basePath,

		client: http.DefaultClient,
		logger: logger,
	}
}

func (c *foodTruckClient) GetEvents(id string, on string) ([]Event, error) {
	var onDay string
	var evr EventsResponse

	if len(id) == 0 {
		return nil, errors.New("Location ID is missing")
	}

	switch on {
	case Tomorrow:
		onDay = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		break
	default:
		onDay = time.Now().Format("2006-01-02")
		break
	}
	qs := map[string]string{
		"include_bookings":    "true",
		"with_active_trucks":  "true",
		"with_booking_status": "approved",
		"on_day":              onDay,
		"for_locations":       id,
	}
	endpoint := fmt.Sprintf("%s://%s%s/%s", c.scheme, c.host, c.basePath, EventsResourcePath)
	c.logger.Infof("Endpoint: %s", endpoint)

	url, err := url.Parse(endpoint)
	if err != nil {
		c.logger.Errorw("Invalid URL", zap.Error(err))
		return nil, err
	}

	// Adding Query Param
	c.logger.Info("Adding query parameters")
	query := url.Query()
	for k, v := range qs {
		c.logger.Infof("Name: %s Value: %s", k, v)
		query.Add(k, v)
	}

	//encode and add to url
	url.RawQuery = query.Encode()

	//setup request
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		c.logger.Errorw("Error setting up http request", zap.Error(err))
		return nil, err
	}

	//call api
	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Errorw("Error calling API", zap.Error(err))
		return nil, err
	}
	c.logger.Info("Reading payload from response")

	p := ws.NewPayload()
	p.ReadResponse(ws.ContentTypeJSON, &evr, resp)

	return evr.Events, nil
}

func (c *foodTruckClient) GetLocation(id string) (Location, error) {
	var l Location

	if len(id) == 0 {
		return l, errors.New("Location ID is missing")
	}

	endpoint := fmt.Sprintf("%s://%s%s/%s", c.scheme, c.host, c.basePath, fmt.Sprintf(LocationResourcePath, id))
	c.logger.Infof("Endpoint: %s", endpoint)

	url, err := url.Parse(endpoint)
	if err != nil {
		c.logger.Errorw("Invalid URL", zap.Error(err))
		return l, err
	}
	//setup request
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		c.logger.Errorw("Error setting up http request", zap.Error(err))
		return l, err
	}

	//call api
	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Errorw("Error calling API", zap.Error(err))
		return l, err
	}
	c.logger.Info("Reading payload from response")

	p := ws.NewPayload()
	p.ReadResponse(ws.ContentTypeJSON, &l, resp)

	return l, nil
}

//EventsResponse is response from events api
type EventsResponse struct {
	Pagination struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
		TotalCount int `json:"total_count"`
	} `json:"pagination"`
	Events []Event `json:"events"`
}

//Event represent an event
type Event struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	EventID     int    `json:"event_id"`
	Bookings    []struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Paid   bool   `json:"paid"`
		Truck  struct {
			Name           string   `json:"name"`
			Trailer        bool     `json:"trailer"`
			FoodCategories []string `json:"food_categories"`
			ID             string   `json:"id"`
			UID            int      `json:"uid"`
			FeaturedPhoto  string   `json:"featured_photo"`
		} `json:"truck"`
	} `json:"bookings"`
	WaitlistEntries []struct {
		ID         int         `json:"id"`
		Expiration interface{} `json:"expiration"`
		Position   int         `json:"position"`
		Truck      struct {
			Slug string `json:"slug"`
		} `json:"truck"`
	} `json:"waitlist_entries"`
}

//Location represents a location where you can find truck
type Location struct {
	Name            string  `json:"name"`
	Longitude       float64 `json:"longitude"`
	Latitude        float64 `json:"latitude"`
	Address         string  `json:"address"`
	Photo           string  `json:"photo"`
	GooglePlaceID   string  `json:"google_place_id"`
	CreatedAt       string  `json:"created_at"`
	NeighborhoodID  int     `json:"neighborhood_id"`
	Slug            string  `json:"slug"`
	FilteredAddress string  `json:"filtered_address"`
	ID              string  `json:"id"`
	UID             int     `json:"uid"`
	Neighborhood    struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	} `json:"neighborhood"`
	Pod struct {
		Name                    string      `json:"name"`
		Slug                    string      `json:"slug"`
		Description             string      `json:"description"`
		LoadInSheet             string      `json:"load_in_sheet"`
		W9Required              bool        `json:"w9_required"`
		CoiRequired             bool        `json:"coi_required"`
		HealthRequired          bool        `json:"health_required"`
		HealthSnohomishRequired interface{} `json:"health_snohomish_required"`
	} `json:"pod"`
}

func trimSpaceAndLower(s string) string {
	r := strings.TrimSpace(s)
	r = strings.ToLower(r)
	return r
}
