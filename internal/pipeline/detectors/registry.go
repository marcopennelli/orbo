package detectors

import (
	"fmt"
	"sync"

	"orbo/internal/pipeline"
)

// Registry manages available detectors
type Registry struct {
	detectors map[string]pipeline.Detector
	mu        sync.RWMutex
}

// NewRegistry creates a new detector registry
func NewRegistry() *Registry {
	return &Registry{
		detectors: make(map[string]pipeline.Detector),
	}
}

// Register adds a detector to the registry
func (r *Registry) Register(detector pipeline.Detector) error {
	if detector == nil {
		return fmt.Errorf("detector cannot be nil")
	}

	name := detector.Name()
	if name == "" {
		return fmt.Errorf("detector name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.detectors[name]; exists {
		return fmt.Errorf("detector %q already registered", name)
	}

	r.detectors[name] = detector
	return nil
}

// Get returns a detector by name
func (r *Registry) Get(name string) (pipeline.Detector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.detectors[name]
	return d, ok
}

// GetAll returns all registered detectors
func (r *Registry) GetAll() []pipeline.Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]pipeline.Detector, 0, len(r.detectors))
	for _, d := range r.detectors {
		result = append(result, d)
	}
	return result
}

// GetHealthy returns only healthy detectors
func (r *Registry) GetHealthy() []pipeline.Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]pipeline.Detector, 0)
	for _, d := range r.detectors {
		if d.IsHealthy() {
			result = append(result, d)
		}
	}
	return result
}

// GetByNames returns detectors matching the given names, in order
func (r *Registry) GetByNames(names []string) []pipeline.Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]pipeline.Detector, 0, len(names))
	for _, name := range names {
		if d, ok := r.detectors[name]; ok {
			result = append(result, d)
		}
	}
	return result
}

// GetHealthyByNames returns healthy detectors matching the given names, in order
func (r *Registry) GetHealthyByNames(names []string) []pipeline.Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]pipeline.Detector, 0, len(names))
	for _, name := range names {
		if d, ok := r.detectors[name]; ok && d.IsHealthy() {
			result = append(result, d)
		}
	}
	return result
}

// Names returns the names of all registered detectors
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.detectors))
	for name := range r.detectors {
		names = append(names, name)
	}
	return names
}

// Unregister removes a detector from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.detectors[name]; !exists {
		return fmt.Errorf("detector %q not found", name)
	}

	delete(r.detectors, name)
	return nil
}

// Close releases all detector resources
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for name, d := range r.detectors {
		if err := d.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("error closing detector %q: %w", name, err)
		}
		delete(r.detectors, name)
	}
	return firstErr
}

// Ensure Registry implements DetectorRegistry
var _ pipeline.DetectorRegistry = (*Registry)(nil)
