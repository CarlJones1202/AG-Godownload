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
}

type GetPerformerResponse struct {
	Data struct {
		FindPerformer StashPerformer `json:"findPerformer"`
	} `json:"data"`
}

func NewStashDBService() *StashDBService {
	endpoint := os.Getenv("STASHDB_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://stashdb.org/graphql"
	}
	return &StashDBService{
		Endpoint: endpoint,
		APIKey:   os.Getenv("STASHDB_API_KEY"),
	}
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
