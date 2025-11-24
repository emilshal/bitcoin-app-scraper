package linkedin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"bitcoinconferencescraper/internal/config"
	"bitcoinconferencescraper/internal/scraper"
)

// Matcher uses a web search API (for example, Google Custom Search)
// to find public LinkedIn profile URLs for attendees.
//
// You must configure a compliant search API and respect its terms
// of service and rate limits.
type Matcher struct {
	httpClient *http.Client

	searchAPIKey   string
	searchEngineID string
	searchDelay    time.Duration
	enabled        bool
}

// NewMatcher constructs a new Matcher instance using the provided HTTP client
// and configuration. If the search API key or engine ID are missing, the
// matcher is disabled and EnrichProfiles will be a no-op.
func NewMatcher(httpClient *http.Client, cfg config.Config) *Matcher {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	enabled := cfg.SearchAPIKey != "" && cfg.SearchEngineID != ""

	return &Matcher{
		httpClient:     httpClient,
		searchAPIKey:   cfg.SearchAPIKey,
		searchEngineID: cfg.SearchEngineID,
		searchDelay:    cfg.SearchDelay,
		enabled:        enabled,
	}
}

// EnrichProfiles attaches LinkedIn URLs to profiles where possible.
//
// For each profile with an empty LinkedInURL, it issues a search query
// like: `"Name" "Company" site:linkedin.com/in` and picks the first
// linkedin.com/in/... result, if any.
func (m *Matcher) EnrichProfiles(ctx context.Context, profiles []scraper.Profile) ([]scraper.Profile, error) {
	if !m.enabled {
		log.Printf("linkedin: search API not configured; skipping LinkedIn enrichment")
		return profiles, nil
	}

	out := make([]scraper.Profile, len(profiles))
	copy(out, profiles)

	for i, p := range out {
		if p.LinkedInURL != "" || strings.TrimSpace(p.Name) == "" {
			continue
		}

		urls, err := m.findLinkedInCandidates(ctx, p)
		if err != nil {
			// Stop on first search error so the caller can
			// persist partial results and optionally resume later.
			return out, fmt.Errorf("search error for %q (%s): %w", p.Name, p.ID, err)
		}
		if len(urls) > 0 {
			// First candidate is used as the primary URL.
			out[i].LinkedInURL = urls[0]
			// Any additional candidates go into PossibleLinkedInURLs.
			if len(urls) > 1 {
				out[i].PossibleLinkedInURLs = urls[1:]
			}
			log.Printf("linkedin: matched %q (%s) -> %s (and %d alternatives)", p.Name, p.ID, urls[0], len(urls)-1)
		} else {
			log.Printf("linkedin: no linkedin.com results for %q (%s)", p.Name, p.ID)
		}

		if m.searchDelay > 0 {
			time.Sleep(m.searchDelay)
		}
	}

	return out, nil
}

// googleSearchResponse is a minimal representation of the Google Custom Search
// JSON API response. Adjust this if you use a different provider.
type googleSearchResponse struct {
	Items []struct {
		Link string `json:"link"`
	} `json:"items"`
}

// findLinkedInCandidates queries the configured search API for candidate
// LinkedIn URLs and returns a slice of linkedin.com/in/... links in the
// order returned by the search engine.
func (m *Matcher) findLinkedInCandidates(ctx context.Context, p scraper.Profile) ([]string, error) {
	name := strings.TrimSpace(p.Name)
	company := strings.TrimSpace(p.Company)

	var queries []string
	if name != "" && company != "" {
		queries = append(queries, fmt.Sprintf("%q %q site:linkedin.com", name, company))
	}
	if name != "" {
		queries = append(queries, fmt.Sprintf("%q site:linkedin.com", name))
		queries = append(queries, fmt.Sprintf("%s site:linkedin.com", name))
	}
	if len(queries) == 0 {
		return nil, nil
	}

	for idx, query := range queries {
		log.Printf("linkedin: querying for %q (%s) with variant %d: %s", p.Name, p.ID, idx+1, query)

		urls, err := m.searchOnce(ctx, query)
		if err != nil {
			return nil, err
		}
		if len(urls) > 0 {
			if idx > 0 {
				log.Printf("linkedin: matches for %q (%s) came from fallback query %d", p.Name, p.ID, idx+1)
			}
			return urls, nil
		}
	}

	return nil, nil
}

func (m *Matcher) searchOnce(ctx context.Context, query string) ([]string, error) {

	u, err := url.Parse("https://www.googleapis.com/customsearch/v1")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	q.Set("key", m.searchAPIKey)
	q.Set("cx", m.searchEngineID)
	q.Set("q", query)
	// Ask for more results to increase the chance
	// of finding a LinkedIn URL.
	q.Set("num", "10")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("search status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var sr googleSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, err
	}

	var personal []string
	var other []string
	for _, item := range sr.Items {
		link := strings.TrimSpace(item.Link)
		if link == "" {
			continue
		}
		if strings.Contains(link, "linkedin.com/in/") {
			personal = append(personal, link)
		} else if strings.Contains(link, "linkedin.com/") {
			other = append(other, link)
		}
	}
	// Prefer personal profile URLs (/in/), but fall back
	// to any linkedin.com URLs if that's all we have.
	return append(personal, other...), nil
}
