package seattlefoodtruck

//Neighborhood represents a neighborhood in seattle where
//foodtrucks can be found
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
