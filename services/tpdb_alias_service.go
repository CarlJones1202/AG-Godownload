package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

const tpdbAPIBaseURL = "https://api.theporndb.net"

type TPDBAliasService struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

type TPDBSiteAlias struct {
	Provider string `json:"provider"`
	Alias    string `json:"alias"`
}

type TPDBPerformerAliases struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Aliases []TPDBSiteAlias `json:"site_aliases"`
}

type tpdbPerformerSearchResponse struct {
	Data []struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Disambiguation string `json:"disambiguation"`
		SitePerformers []struct {
			Name string `json:"name"`
			Site *struct {
				Name string `json:"name"`
			} `json:"site"`
		} `json:"site_performers"`
	} `json:"data"`
}

var tpdbScraperProviders = map[string]string{
	"metart":        "MetArt",
	"metartx":       "MetartX",
	"playboy":       "PlayboyPlus",
	"playboyplus":   "PlayboyPlus",
	"sexart":        "SexArt",
	"lifeerotic":    "LifeErotic",
	"eternaldesire": "EternalDesire",
	"mplstudios":    "MPLStudios",
	"vivthomas":     "VivThomas",
	"wowgirls":      "WowGirls",
	"rylskyart":     "RylskyArt",
}

func canonicalTPDBScraperProvider(name string) (string, bool) {
	key := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, strings.TrimSpace(name))
	provider, ok := tpdbScraperProviders[key]
	return provider, ok
}

func NewTPDBAliasService(apiKey string) *TPDBAliasService {
	return &TPDBAliasService{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: tpdbAPIBaseURL,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *TPDBAliasService) Search(ctx context.Context, name string) ([]TPDBPerformerAliases, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("TPDB API key is not configured")
	}

	endpoint, err := url.Parse(strings.TrimRight(s.baseURL, "/") + "/performers")
	if err != nil {
		return nil, fmt.Errorf("invalid TPDB API URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("q", name)
	query.Set("per_page", "20")
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create TPDB request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request TPDB performer search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("TPDB performer search returned status %d", resp.StatusCode)
	}

	var result tpdbPerformerSearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 5<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode TPDB performer search: %w", err)
	}

	performers := make([]TPDBPerformerAliases, 0, len(result.Data))
	for _, performer := range result.Data {
		match := TPDBPerformerAliases{
			ID:      performer.ID,
			Name:    performer.Name,
			Aliases: make([]TPDBSiteAlias, 0, len(performer.SitePerformers)),
		}
		if performer.Disambiguation != "" {
			match.Name += " (" + performer.Disambiguation + ")"
		}
		for _, sitePerformer := range performer.SitePerformers {
			if sitePerformer.Site == nil || strings.TrimSpace(sitePerformer.Site.Name) == "" ||
				strings.TrimSpace(sitePerformer.Name) == "" {
				continue
			}
			provider, ok := canonicalTPDBScraperProvider(sitePerformer.Site.Name)
			if !ok {
				continue
			}
			match.Aliases = append(match.Aliases, TPDBSiteAlias{
				Provider: provider,
				Alias:    strings.TrimSpace(sitePerformer.Name),
			})
		}
		performers = append(performers, match)
	}
	return performers, nil
}
