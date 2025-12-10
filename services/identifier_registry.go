package services

import (
	"fmt"
	"sync"
)

// IdentifierRegistry manages all identifier providers
type IdentifierRegistry struct {
	providers map[string]IdentifierProvider
	mu        sync.RWMutex
}

var (
	globalRegistry *IdentifierRegistry
	once           sync.Once
)

// GetIdentifierRegistry returns the global identifier registry (singleton)
func GetIdentifierRegistry() *IdentifierRegistry {
	once.Do(func() {
		globalRegistry = &IdentifierRegistry{
			providers: make(map[string]IdentifierProvider),
		}

		// Register StashDB provider
		globalRegistry.Register(NewStashDBService())

		// Register FreeOnes provider
		globalRegistry.Register(NewFreeOnesService())

		// Register Babepedia provider
		globalRegistry.Register(NewBabepediaService())

		// Future: Register other providers here
		// globalRegistry.Register(NewTPDBService())
		// globalRegistry.Register(NewIAFDService())
	})
	return globalRegistry
}

// Register adds a new identifier provider to the registry
func (r *IdentifierRegistry) Register(provider IdentifierProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider.GetName()] = provider
}

// GetProvider returns a specific identifier provider by name
func (r *IdentifierRegistry) GetProvider(name string) (IdentifierProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("identifier provider '%s' not found", name)
	}
	return provider, nil
}

// ListProviders returns a list of all registered provider names
func (r *IdentifierRegistry) ListProviders() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
