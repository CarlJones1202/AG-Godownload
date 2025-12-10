package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type StashDBService struct {
	Endpoint string
	APIKey   string
}

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type StashPerformer struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Disambiguation string   `json:"disambiguation"`
	Aliases        []string `json:"aliases"`
	Gender         string   `json:"gender"`
	Birthdate      struct {
		Date string `json:"date"`
	} `json:"birthdate"`
	Country   string `json:"country"`
	Height    int    `json:"height"`
	HairColor string `json:"hair_color"`
	EyeColor  string `json:"eye_color"`
	Ethnicity string `json:"ethnicity"`
	// Fixed fields based on error
	Measurements struct {
		BandSize int    `json:"band_size"`
		CupSize  string `json:"cup_size"`
		Waist    int    `json:"waist"`
		Hip      int    `json:"hip"`
	} `json:"measurements"`
	FakeTits string `json:"fake_tits"` // Keeping this as string for now, but might need removal if still invalid. Error said "Cannot query field", so I should remove it from query but maybe keep in struct if I want to map it later? No, remove from struct to be safe.
	// Actually, let's remove the invalid fields from struct too to avoid confusion
	Tattoos []struct {
		Location    string `json:"location"`
		Description string `json:"description"`
	} `json:"tattoos"`
	Piercings []struct {
		Location    string `json:"location"`
		Description string `json:"description"`
	} `json:"piercings"`
	// Details field removed as it is invalid
	// Social media usually in urls
	URLs []struct {
		URL  string `json:"url"`
		Type string `json:"type"`
	} `json:"urls"`

	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

type SearchPerformersResponse struct {
	Data struct {
		QueryPerformers struct {
			Performers []StashPerformer `json:"performers"`
		} `json:"queryPerformers"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type GetPerformerResponse struct {
	Data struct {
		FindPerformer StashPerformer `json:"findPerformer"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func NewStashDBService() *StashDBService {
	endpoint := os.Getenv("STASHDB_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://stashdb.org/graphql"
	}
	apiKey := os.Getenv("STASHDB_API_KEY")
	if apiKey != "" {
		fmt.Println("StashDB Service: API Key found")
	} else {
		fmt.Println("StashDB Service: API Key NOT found")
	}
	return &StashDBService{
		Endpoint: endpoint,
		APIKey:   apiKey,
	}
}

// GetName implements IdentifierProvider interface
func (s *StashDBService) GetName() string {
	return "stashdb"
}

// Search implements IdentifierProvider interface
func (s *StashDBService) Search(name string) ([]IdentifierResult, error) {
	performers, err := s.SearchPerformers(name)
	if err != nil {
		return nil, err
	}

	results := make([]IdentifierResult, len(performers))
	for i, p := range performers {
		previewData := map[string]interface{}{
			"gender":    p.Gender,
			"country":   p.Country,
			"birthdate": p.Birthdate.Date,
		}
		if len(p.Images) > 0 {
			previewData["image_url"] = p.Images[0].URL
		}

		results[i] = IdentifierResult{
			ExternalID:     p.ID,
			Name:           p.Name,
			Disambiguation: p.Disambiguation,
			PreviewData:    previewData,
		}
	}
	return results, nil
}

// GetDetails implements IdentifierProvider interface
func (s *StashDBService) GetDetails(externalID string) (*PersonData, error) {
	performer, err := s.GetPerformer(externalID)
	if err != nil {
		return nil, err
	}

	// Extract photo URLs
	photos := make([]string, len(performer.Images))
	for i, img := range performer.Images {
		photos[i] = img.URL
	}

	// Format tattoos
	tattoos := ""
	if len(performer.Tattoos) > 0 {
		tattooStrs := make([]string, len(performer.Tattoos))
		for i, t := range performer.Tattoos {
			tattooStrs[i] = fmt.Sprintf("%s (%s)", t.Description, t.Location)
		}
		tattoos = fmt.Sprintf("%v", tattooStrs)
	}

	// Format piercings
	piercings := ""
	if len(performer.Piercings) > 0 {
		piercingStrs := make([]string, len(performer.Piercings))
		for i, p := range performer.Piercings {
			piercingStrs[i] = fmt.Sprintf("%s (%s)", p.Description, p.Location)
		}
		piercings = fmt.Sprintf("%v", piercingStrs)
	}

	// Extract social media
	twitter := ""
	instagram := ""
	for _, u := range performer.URLs {
		urlLower := fmt.Sprintf("%v", u.URL)
		if twitter == "" && (contains(urlLower, "twitter.com") || contains(urlLower, "x.com")) {
			twitter = extractUsername(u.URL)
		}
		if instagram == "" && contains(urlLower, "instagram.com") {
			instagram = extractUsername(u.URL)
		}
	}

	return &PersonData{
		Name:      performer.Name,
		Aliases:   performer.Aliases,
		Birthdate: performer.Birthdate.Date,
		Country:   performer.Country,
		Ethnicity: performer.Ethnicity,
		EyeColor:  performer.EyeColor,
		HairColor: performer.HairColor,
		Height:    fmt.Sprintf("%d", performer.Height),
		Measurements: fmt.Sprintf("Band: %d, Cup: %s, Waist: %d, Hip: %d",
			performer.Measurements.BandSize, performer.Measurements.CupSize,
			performer.Measurements.Waist, performer.Measurements.Hip),
		Tattoos:   tattoos,
		Piercings: piercings,
		Twitter:   twitter,
		Instagram: instagram,
		Photos:    photos,
		RawData: map[string]interface{}{
			"gender": performer.Gender,
		},
	}, nil
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func extractUsername(url string) string {
	parts := []rune(url)
	var result []rune
	slashCount := 0
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == '/' {
			slashCount++
			if slashCount > 1 {
				break
			}
		} else if slashCount == 1 {
			result = append([]rune{parts[i]}, result...)
		}
	}
	return string(result)
}

func (s *StashDBService) SearchPerformers(name string) ([]StashPerformer, error) {
	query := `
		query SearchPerformers($name: String!) {
			queryPerformers(input: {names: $name, page: 1, per_page: 20}) {
				performers {
					id
					name
					disambiguation
					aliases
					gender
					birthdate {
						date
					}
					country
					height
					hair_color
					eye_color
					ethnicity
					measurements {
						band_size
						cup_size
						waist
						hip
					}
					tattoos {
						location
						description
					}
					piercings {
						location
						description
					}
					urls {
						url
						type
					}
					images {
						url
					}
				}
			}
		}
	`
	variables := map[string]interface{}{
		"name": name,
	}

	var resp SearchPerformersResponse
	if err := s.execute(query, variables, &resp); err != nil {
		return nil, err
	}

	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", resp.Errors[0].Message)
	}

	return resp.Data.QueryPerformers.Performers, nil
}

func (s *StashDBService) GetPerformer(id string) (*StashPerformer, error) {
	query := `
		query GetPerformer($id: ID!) {
			findPerformer(id: $id) {
				id
				name
				disambiguation
				aliases
				gender
				birthdate {
					date
				}
				country
				height
				hair_color
				eye_color
				ethnicity
				measurements {
					band_size
					cup_size
					waist
					hip
				}
				tattoos {
					location
					description
				}
				piercings {
					location
					description
				}
				urls {
					url
					type
				}
				images {
					url
				}
			}
		}
	`
	variables := map[string]interface{}{
		"id": id,
	}

	var resp GetPerformerResponse
	if err := s.execute(query, variables, &resp); err != nil {
		return nil, err
	}

	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", resp.Errors[0].Message)
	}

	return &resp.Data.FindPerformer, nil
}

func (s *StashDBService) execute(query string, variables map[string]interface{}, response interface{}) error {
	reqBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", s.Endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if s.APIKey != "" {
		req.Header.Set("ApiKey", s.APIKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("stashdb api returned status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return err
	}

	return nil
}
