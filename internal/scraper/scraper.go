package scraper

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Scraper orchestrates high-level scraping logic using the Client.
type Scraper struct {
	Client              *Client
	PageSize            int
	EventID             string
	DelayBetweenRequests time.Duration
}

// ScrapeAllProfiles walks over pages until there are no more or maxPages is reached.
// If maxPages <= 0, it keeps going until the API reports no more pages.
func (s Scraper) ScrapeAllProfiles(ctx context.Context, maxPages int) ([]Profile, error) {
	if s.Client == nil {
		return nil, fmt.Errorf("scraper client is nil")
	}
	if s.EventID == "" {
		return nil, fmt.Errorf("event ID is empty")
	}
	if s.PageSize <= 0 {
		s.PageSize = 50
	}
	if s.DelayBetweenRequests < 0 {
		s.DelayBetweenRequests = 0
	}

	var all []Profile
	page := 1

	for {
		if maxPages > 0 && page > maxPages {
			break
		}

		log.Printf("scraper: fetching page %d (page size %d)", page, s.PageSize)

		res, err := s.Client.ListProfiles(ctx, s.EventID, page, s.PageSize)
		if err != nil {
			return nil, fmt.Errorf("listing profiles page %d: %w", page, err)
		}

		if len(res.Profiles) == 0 {
			log.Printf("scraper: page %d returned 0 attendees, stopping", page)
			break
		}

		log.Printf("scraper: page %d returned %d attendee ids", page, len(res.Profiles))

		for _, stub := range res.Profiles {
			if stub.ID == "" {
				continue
			}

			log.Printf("scraper: fetching attendee %s", stub.ID)

			profile, err := s.Client.GetAttendeeProfile(ctx, s.EventID, stub.ID)
			if err != nil {
				return nil, fmt.Errorf("getting attendee %s: %w", stub.ID, err)
			}

			all = append(all, profile)

			if s.DelayBetweenRequests > 0 {
				time.Sleep(s.DelayBetweenRequests)
			}
		}

		if !res.HasNext {
			break
		}

		page++
	}

	log.Printf("scraper: finished, collected %d profiles", len(all))

	return all, nil
}
