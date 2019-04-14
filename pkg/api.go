package pkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	//Today for today
	Today = "today"

	//Tomorrow for tomorrow
	Tomorrow = "tomorrow"
)

//API is seattle foodtruck api
type API struct {
	Host     string
	Scheme   string
	BasePath string

	HttpClient *http.Client
}

//FindTrucks searches for booked foodtrucks at seattle foodtruck site
func (a *API) FindTrucks(neighborhood string, locations []string, at string) ([]*Location, error) {
	var onDay string

	switch at {
	case Tomorrow:
		onDay = time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		break
	default:
		onDay = time.Now().Format("2006-01-02")
		break
	}
	path := "locations"
	qs := map[string]string{
		"only_with_events":   "true",
		"with_active_trucks": "true",
		"include_events":     "true",
		"include_trucks":     "true",
		"with_events_on_day": onDay,
		"neighborhood":       neighborhood,
	}
	endpoint := fmt.Sprintf("%s://%s%s/%s", a.Scheme, a.Host, a.BasePath, path)

	url, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	// Adding Query Param
	query := url.Query()
	for k, v := range qs {
		query.Add(k, v)
	}

	//encode and add to url
	url.RawQuery = query.Encode()

	//setup request
	httpRequest, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, err
	}
	//call api
	httpResponse, err := a.HttpClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	responsePayload, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}
	var apiResponse LocationsAPIResponse
	if err := json.Unmarshal(responsePayload, &apiResponse); err != nil {
		return nil, err
	}
	var ret []*Location

	for _, locationID := range locations {
		if l := contains(apiResponse.Locations, locationID); l != nil {
			ret = append(ret, l)
		}
	}

	return ret, nil
}

//GetLocation returns location for identifier passed
func (a *API) GetLocation(id string) (*Location, error) {
	if len(id) == 0 {
		return nil, errors.New("Location id is required")
	}
	path := fmt.Sprintf("/locations/%s", id)
	endpoint := fmt.Sprintf("%s://%s%s/%s", a.Scheme, a.Host, a.BasePath, path)

	//setup request
	httpRequest, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	//call api
	httpResponse, err := a.HttpClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	responsePayload, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}
	var ret Location
	if err := json.Unmarshal(responsePayload, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

//GetNeighborhood returns neighborhood for identifier passed
func (a *API) GetNeighborhood(id int) (*Neighborhood, error) {
	if id == 0 {
		return nil, errors.New("Neighborhood id is required")
	}
	path := fmt.Sprintf("/neighborhoods/%v", id)
	endpoint := fmt.Sprintf("%s://%s%s/%s", a.Scheme, a.Host, a.BasePath, path)

	//setup request
	httpRequest, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	//call api
	httpResponse, err := a.HttpClient.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	responsePayload, err := ioutil.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, err
	}
	var ret Neighborhood
	if err := json.Unmarshal(responsePayload, &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

//LocationsAPIResponse represents a response from locations api that returns locations served
//in neighborhood served with event bookings for a today or tomorrow
type LocationsAPIResponse struct {
	Pagination struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
		TotalCount int `json:"total_count"`
	} `json:"pagination"`
	Locations []*Location `json:"locations"`
}

//Location represents a location in neighborhood
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
	} `json:"pod,omitempty"`
	Events []struct {
		Name      string `json:"name"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
		ID        int    `json:"id"`
		EventID   int    `json:"event_id"`
		Trucks    []struct {
			Name           string   `json:"name"`
			ID             string   `json:"id"`
			FoodCategories []string `json:"food_categories"`
			FeaturedPhoto  string   `json:"featured_photo"`
		} `json:"trucks"`
	} `json:"events"`
}

//Neighborhood is neighborhood served by seattlefoodtruck
type Neighborhood struct {
	Name        string `json:"name"`
	Latitude    string `json:"latitude"`
	Longitude   string `json:"longitude"`
	Description string `json:"description"`
	ZoomLevel   int    `json:"zoom_level"`
	Photo       string `json:"photo"`
	ID          string `json:"id"`
	UID         int    `json:"uid"`
}

func trimSpaceAndLower(s string) string {
	r := strings.TrimSpace(s)
	r = strings.ToLower(r)
	return r
}

func contains(locs []*Location, id string) *Location {
	for _, l := range locs {
		locID := trimSpaceAndLower(l.ID)
		id = trimSpaceAndLower(id)

		if locID == id {
			return l
		}
	}
	return nil
}
