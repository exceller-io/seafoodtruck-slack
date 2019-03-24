package seattlefoodtruck

import "net/http"

//Configuration represents config for seattlefoodtruck api
type Configuration struct {
	BasePath      string            `json:"basePath,omitempty"`
	Host          string            `json:"host,omitempty"`
	Scheme        string            `json:"scheme,omitempty"`
	DefaultHeader map[string]string `json:"defaultHeader,omitempty"`
	UserAgent     string            `json:"userAgent,omitempty"`
	HTTPClient    *http.Client
}

//NewConfiguration returns an instance of Configuration
func NewConfiguration() *Configuration {
	cfg := &Configuration{
		BasePath:      "https://www.seattlefoodtruck.com/api",
		DefaultHeader: make(map[string]string),
		UserAgent:     "seafoodtruck-slack/1.0.0/go",
	}
	return cfg
}

//AddDefaultHeader adds default header
func (c *Configuration) AddDefaultHeader(key string, value string) {
	c.DefaultHeader[key] = value
}
