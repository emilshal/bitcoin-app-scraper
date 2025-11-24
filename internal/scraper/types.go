package scraper

// Profile represents a user profile from the Bitcoin Conference app.
// Fields can be expanded as you discover them in the API responses.
type Profile struct {
	ID                   string   `json:"id,omitempty"`
	Name                 string   `json:"name"`
	Title                string   `json:"title,omitempty"`
	Company              string   `json:"company,omitempty"`
	Location             string   `json:"location,omitempty"`
	LinkedInURL          string   `json:"linkedin_url"`
	PossibleLinkedInURLs []string `json:"possible_linkedin_urls,omitempty"`
}
