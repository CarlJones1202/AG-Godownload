package services

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	// Add other fields as needed
}

type SearchPerformersResponse struct {
	Data struct {
		FindPerformers struct {
			Performers []StashPerformer `json:"performers"`
		} `json:"findPerformers"`
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
			findPerformers(performer_filter: {name: {value: $name, modifier: CONTAINS}}) {
				performers {
					id
					name
					aliases
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

	return resp.Data.FindPerformers.Performers, nil
}

func (s *StashDBService) GetPerformer(id string) (*StashPerformer, error) {
	query := `
		query GetPerformer($id: ID!) {
			findPerformer(id: $id) {
				id
				name
				aliases
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
		return fmt.Errorf("stashdb api returned status: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return err
	}

	return nil
}
