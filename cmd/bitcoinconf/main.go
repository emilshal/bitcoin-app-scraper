package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"bitcoinconferencescraper/internal/config"
	"bitcoinconferencescraper/internal/linkedin"
	"bitcoinconferencescraper/internal/scraper"
)

func main() {
	var (
		outputPath = flag.String("out", "profiles.json", "output file path (JSON)")
		inputPath  = flag.String("in", "", "optional input file path (JSON) with existing profiles; if set, scraping is skipped")
		pageLimit  = flag.Int("page-limit", 0, "maximum number of pages to scrape (0 = all)")
		pageSize   = flag.Int("page-size", 50, "number of profiles per page when calling the API")
		timeoutSec = flag.Int("timeout-sec", 30, "HTTP client timeout in seconds")
	)

	flag.Parse()

	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	httpClient := config.NewHTTPClient(time.Duration(*timeoutSec) * time.Second)

	apiClient := scraper.NewClient(cfg.APIBaseURL, cfg.AuthToken, httpClient)
	apiClient.AccessToken = cfg.AccessToken
	apiClient.ClientID = cfg.ClientID
	apiClient.UID = cfg.UID
	apiClient.SessionCookie = cfg.SessionCookie
	apiClient.BrellaMediaType = cfg.BrellaMediaType

	ctx := context.Background()

	var profiles []scraper.Profile

	if *inputPath != "" {
		log.Printf("loading existing profiles from %s (skipping Brella scraping)", *inputPath)
		profiles, err = readProfilesJSON(*inputPath)
		if err != nil {
			log.Fatalf("read input error: %v", err)
		}
	} else {
		profileScraper := scraper.Scraper{
			Client:               apiClient,
			PageSize:             *pageSize,
			EventID:              cfg.EventID,
			DelayBetweenRequests: cfg.RequestDelay,
		}

		profiles, err = profileScraper.ScrapeAllProfiles(ctx, *pageLimit)
		if err != nil {
			log.Fatalf("scrape error: %v", err)
		}
	}

	linkedinMatcher := linkedin.NewMatcher(httpClient, cfg)
	profiles, err = linkedinMatcher.EnrichProfiles(ctx, profiles)
	if err != nil {
		log.Printf("linkedin matching error: %v", err)
		log.Printf("writing partial results to %s after error", *outputPath)
		if writeErr := writeProfilesJSON(*outputPath, profiles); writeErr != nil {
			log.Fatalf("write output error after linkedin error: %v", writeErr)
		}
		os.Exit(1)
	}

	if err := writeProfilesJSON(*outputPath, profiles); err != nil {
		log.Fatalf("write output error: %v", err)
	}

	fmt.Printf("wrote %d profiles to %s\n", len(profiles), *outputPath)
}

func writeProfilesJSON(path string, profiles []scraper.Profile) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(profiles)
}

func readProfilesJSON(path string) ([]scraper.Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var profiles []scraper.Profile
	if err := json.NewDecoder(f).Decode(&profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}
