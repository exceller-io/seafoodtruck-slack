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

	//TruckResourcePath represents path to retrieve truck
	TruckResourcePath = "trucks/%s"
)

//FoodTruckClient represents generic interface for Seattle FoodTruck API client
type FoodTruckClient interface {
	GetEvents(id string, onDay string) ([]Event, error)
	GetLocation(id string) (Location, error)
	GetTruck(id string) (Truck, error)
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

	callAPI(endpoint, qs, c.client, &evr)

	return evr.Events, nil
}

func (c *foodTruckClient) GetLocation(id string) (Location, error) {
	var l Location

	if len(id) == 0 {
		return l, errors.New("Location ID is missing")
	}
	endpoint := fmt.Sprintf("%s://%s%s/%s", c.scheme, c.host, c.basePath, fmt.Sprintf(LocationResourcePath, id))
	c.logger.Infof("Endpoint: %s", endpoint)

	callAPI(endpoint, nil, c.client, &l)

	return l, nil
}

func (c *foodTruckClient) GetTruck(id string) (Truck, error) {
	var t Truck
	if len(id) == 0 {
		return t, errors.New("Truck ID is required")
	}
	endpoint := fmt.Sprintf("%s://%s%s/%s", c.scheme, c.host, c.basePath, fmt.Sprintf(TruckResourcePath, id))
	c.logger.Infof("Endpoint: %s", endpoint)

	callAPI(endpoint, nil, c.client, &t)

	return t, nil
}

func callAPI(endPoint string, qs map[string]string, client *http.Client, data interface{}) error {
	url, err := url.Parse(endPoint)
	if err != nil {
		return err
	}
	if qs != nil {
		query := url.Query()
		for k, v := range qs {
			query.Add(k, v)
		}
		//encode and add to url
		url.RawQuery = query.Encode()
	}
	//setup request
	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return err
	}

	//call api
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	p := ws.NewPayload()
	p.ReadResponse(ws.ContentTypeJSON, &data, resp)

	return nil
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

//Truck represents a food truck
type Truck struct {
	Name                      string      `json:"name"`
	Rating                    float64     `json:"rating"`
	UserID                    int         `json:"user_id"`
	Featured                  bool        `json:"featured"`
	RatingCount               int         `json:"rating_count"`
	ID                        string      `json:"id"`
	UID                       int         `json:"uid"`
	FeaturedPhoto             string      `json:"featured_photo"`
	Facebook                  string      `json:"facebook"`
	CreatedAt                 string      `json:"created_at"`
	UpdatedAt                 string      `json:"updated_at"`
	Twitter                   string      `json:"twitter"`
	Instagram                 string      `json:"instagram"`
	Yelp                      string      `json:"yelp"`
	Description               string      `json:"description"`
	Phone                     string      `json:"phone"`
	Email                     string      `json:"email"`
	Website                   string      `json:"website"`
	Active                    bool        `json:"active"`
	ContactName               string      `json:"contact_name"`
	TruckLength               int         `json:"truck_length"`
	TruckWidth                int         `json:"truck_width"`
	Trailer                   bool        `json:"trailer"`
	AcceptsCreditCards        bool        `json:"accepts_credit_cards"`
	GlutenFree                bool        `json:"gluten_free"`
	Vegetarian                bool        `json:"vegetarian"`
	Vegan                     bool        `json:"vegan"`
	Paleo                     bool        `json:"paleo"`
	FutureBookings            int         `json:"future_bookings"`
	FuturePodEvents           int         `json:"future_pod_events"`
	Coi                       string      `json:"coi"`
	CoiExpiration             string      `json:"coi_expiration"`
	CoiStatus                 string      `json:"coi_status"`
	CoiApproved               bool        `json:"coi_approved"`
	Health                    string      `json:"health"`
	HealthExpiration          string      `json:"health_expiration"`
	HealthStatus              string      `json:"health_status"`
	HealthApproved            bool        `json:"health_approved"`
	W9                        string      `json:"w9"`
	W9Expiration              interface{} `json:"w9_expiration"`
	W9Status                  string      `json:"w9_status"`
	W9Approved                bool        `json:"w9_approved"`
	HealthSnohomish           string      `json:"health_snohomish"`
	HealthSnohomishExpiration string      `json:"health_snohomish_expiration"`
	HealthSnohomishStatus     string      `json:"health_snohomish_status"`
	HealthSnohomishApproved   bool        `json:"health_snohomish_approved"`
	MenuItems                 []struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
		ID          int     `json:"id"`
	} `json:"menu_items"`
	Photos []struct {
		ID       int    `json:"id"`
		File     string `json:"file"`
		Position int    `json:"position"`
	} `json:"photos"`
	RelatedTrucks []struct {
		Name           string  `json:"name"`
		Rating         float64 `json:"rating"`
		RatingCount    int     `json:"rating_count"`
		ID             string  `json:"id"`
		FeaturedPhoto  string  `json:"featured_photo"`
		FoodCategories []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
			UID  int    `json:"uid"`
		} `json:"food_categories"`
	} `json:"related_trucks"`
	FoodCategories []struct {
		Name string `json:"name"`
		ID   string `json:"id"`
		UID  int    `json:"uid"`
	} `json:"food_categories"`
}

func trimSpaceAndLower(s string) string {
	r := strings.TrimSpace(s)
	r = strings.ToLower(r)
	return r
}
