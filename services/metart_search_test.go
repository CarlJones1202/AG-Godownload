package services

import (
	"testing"
)

func TestModelMatchesSearchTerm(t *testing.T) {
	tests := []struct {
		name       string
		models     []string
		searchTerm string
		want       bool
	}{
		{
			name:       "exact match",
			models:     []string{"Aria Noor"},
			searchTerm: "Aria Noor",
			want:       true,
		},
		{
			name:       "first name matches, last does not",
			models:     []string{"Aria Sloane"},
			searchTerm: "Aria Noor",
			want:       false,
		},
		{
			name:       "last name matches, first does not",
			models:     []string{"Sky Moon"},
			searchTerm: "Aria Sky",
			want:       false,
		},
		{
			name:       "inverted name order",
			models:     []string{"Noor, Aria"},
			searchTerm: "Aria Noor",
			want:       true,
		},
		{
			name:       "hyphenated model name",
			models:     []string{"Aria-Noor"},
			searchTerm: "Aria Noor",
			want:       true,
		},
		{
			name:       "model with disambiguator suffix",
			models:     []string{"Aria Noor (A)"},
			searchTerm: "Aria Noor",
			want:       true,
		},
		{
			name:       "multiple models in gallery, one matches",
			models:     []string{"Elena Koshka", "Aria Noor"},
			searchTerm: "Aria Noor",
			want:       true,
		},
		{
			name:       "multiple models in gallery, none match",
			models:     []string{"Elena Koshka", "Aria Sloane"},
			searchTerm: "Aria Noor",
			want:       false,
		},
		{
			name:       "substring in longer name should NOT match (Martha vs Art)",
			models:     []string{"Martha Smith"},
			searchTerm: "Art Smith",
			want:       false,
		},
		{
			name:       "substring in longer name should NOT match (Geneva vs Eva)",
			models:     []string{"Geneva Green"},
			searchTerm: "Eva Green",
			want:       false,
		},
		{
			name:       "mononym exact match",
			models:     []string{"Libby"},
			searchTerm: "Libby",
			want:       true,
		},
		{
			name:       "mononym with single letter suffix",
			models:     []string{"Libby A"},
			searchTerm: "Libby",
			want:       true,
		},
		{
			name:       "mononym search should NOT match multi-word model",
			models:     []string{"Libby Jones"},
			searchTerm: "Libby",
			want:       false,
		},
		{
			name:       "empty models list allows result through (benefit of doubt)",
			models:     []string{},
			searchTerm: "Aria Noor",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := GallerySearchResult{
				Models: tt.models,
			}
			got := modelMatchesSearchTerm(res, tt.searchTerm)
			if got != tt.want {
				t.Errorf("modelMatchesSearchTerm() = %v, want %v (models=%v, search=%s)", got, tt.want, tt.models, tt.searchTerm)
			}
		})
	}
}
