// Package secret defines the boundary between product use cases and credential storage.
package secret

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// Adapter stores credential values outside product persistence and returns opaque references.
type Adapter interface {
	Store(ctx context.Context, name, value string) (string, error)
	Resolve(ctx context.Context, reference string) (string, error)
	Delete(ctx context.Context, reference string) error
}

// MemoryAdapter is a process-local Secret Adapter for tests and Browser-like hosts.
type MemoryAdapter struct {
	mu      sync.RWMutex
	secrets map[string]string
}

// NewMemoryAdapter returns an empty process-local Secret Adapter.
func NewMemoryAdapter() *MemoryAdapter {
	return &MemoryAdapter{secrets: map[string]string{}}
}

// Store saves a Secret under a stable opaque reference.
func (a *MemoryAdapter) Store(_ context.Context, name, value string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("store secret: name must not be empty")
	}
	if value == "" {
		return "", fmt.Errorf("store secret: value must not be empty")
	}
	reference := "memory://gum-workflows/" + url.PathEscape(name)
	a.mu.Lock()
	a.secrets[reference] = value
	a.mu.Unlock()
	return reference, nil
}

// Resolve returns the Secret named by an opaque memory reference.
func (a *MemoryAdapter) Resolve(_ context.Context, reference string) (string, error) {
	a.mu.RLock()
	value, ok := a.secrets[reference]
	a.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("resolve secret: reference not found")
	}
	return value, nil
}

// Delete removes a Secret named by an opaque memory reference.
func (a *MemoryAdapter) Delete(_ context.Context, reference string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.secrets[reference]; !ok {
		return fmt.Errorf("delete secret: reference not found")
	}
	delete(a.secrets, reference)
	return nil
}
