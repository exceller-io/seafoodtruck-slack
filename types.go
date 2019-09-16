package main

type markdown struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type accessory struct {
	Type     string `json:"type"`
	ImageURL string `json:"image_url"`
	AltText  string `json:"alt_text"`
}

type item struct {
	Type string   `json:"type"`
	Text markdown `json:"text,omitempty"`
}
