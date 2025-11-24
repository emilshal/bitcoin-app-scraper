package config

import (
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// APIBaseURL is the base URL of the backend API.
	// For the Brella example, this would be:
	//   https://api.brella.io
	APIBaseURL string

	// EventID identifies the specific event whose attendees you are scraping.
	// For example: AMS25.
	EventID string

	// AuthToken is an optional auth token or API key if required by the API.
	AuthToken string

	// AccessToken, ClientID, and UID are optional Brella auth headers
	// (commonly used with token-based auth on api.brella.io).
	// If you see these headers on authorized requests in Proxyman,
	// copy their values into the corresponding environment variables.
	AccessToken string
	ClientID    string
	UID         string

	// SessionCookie is an optional _brella_session cookie value, if needed.
	SessionCookie string

	// BrellaMediaType is sent as x-brella-media-type; defaults to brella.latest
	// if unset.
	BrellaMediaType string

	// RequestDelay is the pause between API requests, used to avoid
	// hammering the Brella backend. Default is 1s.
	RequestDelay time.Duration

	// SearchAPIKey and SearchEngineID are used for the web search API
	// (for example, Google Custom Search) to look up public LinkedIn URLs.
	// Both must be set for LinkedIn enrichment to run.
	SearchAPIKey   string
	SearchEngineID string

	// SearchDelay is the pause between search API requests.
	SearchDelay time.Duration
}

// FromEnv loads configuration from environment variables.
func FromEnv() (Config, error) {
	baseURL := os.Getenv("BITCONF_API_BASE_URL")
	if baseURL == "" {
		return Config{}, errors.New("BITCONF_API_BASE_URL is not set")
	}

	eventID := os.Getenv("BITCONF_EVENT_ID")
	if eventID == "" {
		return Config{}, errors.New("BITCONF_EVENT_ID is not set")
	}

	authToken := os.Getenv("BITCONF_API_AUTH_TOKEN")

	accessToken := os.Getenv("BITCONF_ACCESS_TOKEN")
	clientID := os.Getenv("BITCONF_CLIENT")
	uid := os.Getenv("BITCONF_UID")
	sessionCookie := os.Getenv("BITCONF_SESSION_COOKIE")

	brellaMediaType := os.Getenv("BITCONF_BRELLA_MEDIA_TYPE")
	if brellaMediaType == "" {
		brellaMediaType = "brella.latest"
	}

	var requestDelay time.Duration
	if d := os.Getenv("BITCONF_REQUEST_DELAY_MS"); d != "" {
		if ms, err := strconv.Atoi(d); err == nil && ms >= 0 {
			requestDelay = time.Duration(ms) * time.Millisecond
		}
	}
	if requestDelay == 0 {
		requestDelay = 1000 * time.Millisecond
	}

	searchAPIKey := os.Getenv("BITCONF_SEARCH_API_KEY")
	searchEngineID := os.Getenv("BITCONF_SEARCH_ENGINE_ID")

	var searchDelay time.Duration
	if d := os.Getenv("BITCONF_SEARCH_DELAY_MS"); d != "" {
		if ms, err := strconv.Atoi(d); err == nil && ms >= 0 {
			searchDelay = time.Duration(ms) * time.Millisecond
		}
	}
	if searchDelay == 0 {
		searchDelay = 1000 * time.Millisecond
	}

	return Config{
		APIBaseURL:      baseURL,
		EventID:         eventID,
		AuthToken:       authToken,
		AccessToken:     accessToken,
		ClientID:        clientID,
		UID:             uid,
		SessionCookie:   sessionCookie,
		BrellaMediaType: brellaMediaType,
		RequestDelay:    requestDelay,
		SearchAPIKey:    searchAPIKey,
		SearchEngineID:  searchEngineID,
		SearchDelay:     searchDelay,
	}, nil
}

// NewHTTPClient returns an HTTP client with reasonable defaults for scraping.
func NewHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
