package services

// IdentifierProvider defines the interface for person identifier sources
type IdentifierProvider interface {
	// GetName returns the provider name (e.g., "stashdb", "tpdb")
	GetName() string

	// Search searches for people by name
	Search(name string) ([]IdentifierResult, error)

	// GetDetails fetches full details for a person by their external ID
	GetDetails(externalID string) (*PersonData, error)
}

// IdentifierResult represents a search result from an identifier provider
type IdentifierResult struct {
	ExternalID     string                 `json:"external_id"`
	Name           string                 `json:"name"`
	Disambiguation string                 `json:"disambiguation"`
	PreviewData    map[string]interface{} `json:"preview_data"` // For displaying in search results
}

// PersonData represents complete person information from an identifier provider
type PersonData struct {
	Name         string                 `json:"name"`
	Aliases      []string               `json:"aliases"`
	Birthdate    string                 `json:"birthdate"`
	Country      string                 `json:"country"`
	Ethnicity    string                 `json:"ethnicity"`
	EyeColor     string                 `json:"eye_color"`
	HairColor    string                 `json:"hair_color"`
	Height       string                 `json:"height"`
	Measurements string                 `json:"measurements"`
	FakeTits     string                 `json:"fake_tits"`
	CareerLength string                 `json:"career_length"`
	Tattoos      string                 `json:"tattoos"`
	Piercings    string                 `json:"piercings"`
	Bio          string                 `json:"bio"`
	Twitter      string                 `json:"twitter"`
	Instagram    string                 `json:"instagram"`
	Photos       []string               `json:"photos"`
	RawData      map[string]interface{} `json:"raw_data"` // Store source-specific data
}
