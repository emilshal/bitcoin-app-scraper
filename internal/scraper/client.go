package scraper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client wraps HTTP access to the Bitcoin Conference API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client

	// AuthToken is used for Authorization: Bearer <token>, if set.
	AuthToken string

	// Optional Brella-specific auth headers.
	AccessToken     string
	ClientID        string
	UID             string
	SessionCookie   string
	BrellaMediaType string
}

// NewClient constructs a new API client.
//
// BaseURL should be the scheme + host (and optional base path) you discover
// in Proxyman when the app calls its backend, for example:
//   https://api.bitcoinconference.com/v1
func NewClient(baseURL, authToken string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		BaseURL:    baseURL,
		HTTPClient: httpClient,
		AuthToken:  authToken,
	}
}

// ListProfilesResult represents one page of profiles and pagination info.
// For the Brella integration, Profiles will initially only contain IDs;
// detailed fields are filled by subsequent per-attendee requests.
type ListProfilesResult struct {
	Profiles []Profile
	HasNext  bool
}

// brellaAttendeesListResponse models the minimal fields we need from the
// attendees list endpoint: just the attendee IDs.
type brellaAttendeesListResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// brellaAttendeeDetailResponse models the structure of the per-attendee
// detail endpoint, focusing on the attendee's user information.
type brellaAttendeeDetailResponse struct {
	Data struct {
		ID             string `json:"id"`
		Type           string `json:"type"`
		Relationships  struct {
			User struct {
				Data struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"data"`
			} `json:"user"`
		} `json:"relationships"`
	} `json:"data"`
	Included []struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			FirstName        string   `json:"first-name"`
			LastName         string   `json:"last-name"`
			CompanyTitle     string   `json:"company-title"`
			CompanyName      string   `json:"company-name"`
			LinkedIn         string   `json:"linkedin"`
			Twitter          string   `json:"twitter"`
			Website          string   `json:"website"`
			TimeZone         string   `json:"time-zone"`
			CompanyCountries []string `json:"company-countries"`
		} `json:"attributes"`
	} `json:"included"`
}

// ListProfiles calls the Brella attendees endpoint for a specific event and page.
//
// Example endpoint (URL-encoded brackets removed for clarity):
//   GET /api/events/{eventID}/attendees
//       ?ignore_networking=true
//       &order=newest
//       &page[number]={page}
//       &page[size]={pageSize}
//       &search=
//
// The HasNext flag is inferred heuristically: if the API returns fewer than
// pageSize attendees, we assume there are no more pages.
func (c *Client) ListProfiles(ctx context.Context, eventID string, page, pageSize int) (ListProfilesResult, error) {
	if eventID == "" {
		return ListProfilesResult{}, errors.New("eventID is empty")
	}

	path := fmt.Sprintf(
		"/api/events/%s/attendees?ignore_networking=true&order=newest&page[number]=%d&page[size]=%d&search=",
		eventID,
		page,
		pageSize,
	)

	req, err := c.newRequest(ctx, http.MethodGet, path)
	if err != nil {
		return ListProfilesResult{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return ListProfilesResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return ListProfilesResult{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var apiResp brellaAttendeesListResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return ListProfilesResult{}, fmt.Errorf("decoding attendees response: %w", err)
	}

	profiles := make([]Profile, 0, len(apiResp.Data))
	for _, item := range apiResp.Data {
		if item.ID == "" {
			continue
		}
		profiles = append(profiles, Profile{
			ID: item.ID,
		})
	}

	hasNext := len(apiResp.Data) == pageSize

	return ListProfilesResult{
		Profiles: profiles,
		HasNext:  hasNext,
	}, nil
}

// GetAttendeeProfile fetches detailed profile data for a single attendee.
func (c *Client) GetAttendeeProfile(ctx context.Context, eventID, attendeeID string) (Profile, error) {
	if eventID == "" {
		return Profile{}, errors.New("eventID is empty")
	}
	if attendeeID == "" {
		return Profile{}, errors.New("attendeeID is empty")
	}

	path := fmt.Sprintf("/api/events/%s/attendees/%s", eventID, attendeeID)

	req, err := c.newRequest(ctx, http.MethodGet, path)
	if err != nil {
		return Profile{}, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Profile{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Profile{}, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var apiResp brellaAttendeeDetailResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return Profile{}, fmt.Errorf("decoding attendee detail: %w", err)
	}

	return mapBrellaDetailToProfile(apiResp), nil
}

// newRequest is a helper to build an HTTP request with auth headers, etc.
func (c *Client) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	if c.BaseURL == "" {
		return nil, errors.New("client BaseURL is empty")
	}

	url := fmt.Sprintf("%s%s", c.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}

	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}
	if c.AccessToken != "" {
		req.Header.Set("access-token", c.AccessToken)
	}
	if c.ClientID != "" {
		req.Header.Set("client", c.ClientID)
	}
	if c.UID != "" {
		req.Header.Set("uid", c.UID)
	}
	if c.BrellaMediaType != "" {
		req.Header.Set("x-brella-media-type", c.BrellaMediaType)
	}
	if c.SessionCookie != "" {
		// Expect just the cookie value here, not the full Set-Cookie string.
		req.Header.Add("Cookie", "_brella_session="+c.SessionCookie)
	}

	// Use the vendor-specific media type expected by Brella.
	req.Header.Set("Accept", "application/vnd.brella.v4+json")
	return req, nil
}
// mapBrellaDetailToProfile converts a detailed attendee response into a Profile.
func mapBrellaDetailToProfile(resp brellaAttendeeDetailResponse) Profile {
	profile := Profile{
		ID: resp.Data.ID,
	}

	userID := resp.Data.Relationships.User.Data.ID
	if userID == "" {
		return profile
	}

	for _, inc := range resp.Included {
		if inc.Type != "user" || inc.ID != userID {
			continue
		}

		first := strings.TrimSpace(inc.Attributes.FirstName)
		last := strings.TrimSpace(inc.Attributes.LastName)
		name := strings.TrimSpace(strings.Join([]string{first, last}, " "))

		location := ""
		if len(inc.Attributes.CompanyCountries) > 0 {
			location = strings.Join(inc.Attributes.CompanyCountries, ", ")
		} else if inc.Attributes.TimeZone != "" {
			location = inc.Attributes.TimeZone
		}

		profile.Name = name
		profile.Title = inc.Attributes.CompanyTitle
		profile.Company = inc.Attributes.CompanyName
		profile.Location = location
		profile.LinkedInURL = inc.Attributes.LinkedIn

		break
	}

	return profile
}
