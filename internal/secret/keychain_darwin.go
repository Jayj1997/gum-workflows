//go:build darwin

package secret

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

const keychainService = "com.gum-workflows.llm-provider"

// KeychainBackend is the injectable macOS Keychain system boundary.
type KeychainBackend interface {
	Store(ctx context.Context, service, account, value string) error
	Resolve(ctx context.Context, service, account string) (string, error)
	Delete(ctx context.Context, service, account string) error
}

// KeychainAdapter stores Provider credentials as generic passwords in the user's macOS Keychain.
type KeychainAdapter struct {
	backend KeychainBackend
}

type sanitizedKeychainError struct {
	cause error
}

func (e sanitizedKeychainError) Error() string { return "macOS Keychain unavailable" }
func (e sanitizedKeychainError) Unwrap() error { return e.cause }

// NewKeychainAdapter creates a macOS Keychain Secret Adapter. A nil backend uses Security.framework.
func NewKeychainAdapter(backend KeychainBackend) *KeychainAdapter {
	if backend == nil {
		backend = newSecurityFrameworkBackend()
	}
	return &KeychainAdapter{backend: backend}
}

// Store creates or replaces one Provider API Key and returns its opaque Keychain reference.
func (a *KeychainAdapter) Store(ctx context.Context, name, value string) (string, error) {
	account, err := providerAccount(name)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("store secret: value must not be empty")
	}
	if err := a.backend.Store(ctx, keychainService, account, value); err != nil {
		return "", fmt.Errorf("store secret: %w", sanitizedKeychainError{cause: err})
	}
	return keychainReference(account), nil
}

// Resolve reads one Provider API Key from the user's macOS Keychain.
func (a *KeychainAdapter) Resolve(ctx context.Context, reference string) (string, error) {
	account, err := parseKeychainReference(reference)
	if err != nil {
		return "", err
	}
	value, err := a.backend.Resolve(ctx, keychainService, account)
	if err != nil {
		return "", fmt.Errorf("resolve secret: %w", sanitizedKeychainError{cause: err})
	}
	return value, nil
}

// Delete removes one Provider API Key from the user's macOS Keychain.
func (a *KeychainAdapter) Delete(ctx context.Context, reference string) error {
	account, err := parseKeychainReference(reference)
	if err != nil {
		return err
	}
	if err := a.backend.Delete(ctx, keychainService, account); err != nil {
		return fmt.Errorf("delete secret: %w", sanitizedKeychainError{cause: err})
	}
	return nil
}

func providerAccount(name string) (string, error) {
	const prefix = "llm-provider/"
	if !strings.HasPrefix(name, prefix) || strings.TrimPrefix(name, prefix) == "" || strings.Contains(strings.TrimPrefix(name, prefix), "/") {
		return "", fmt.Errorf("store secret: name must identify one LLM Provider")
	}
	return strings.TrimPrefix(name, prefix), nil
}

func keychainReference(account string) string {
	return "keychain://" + keychainService + "/" + url.PathEscape(account)
}

func parseKeychainReference(reference string) (string, error) {
	parsed, err := url.Parse(reference)
	if err != nil || parsed.Scheme != "keychain" || parsed.Host != keychainService || parsed.Path == "" {
		return "", fmt.Errorf("invalid macOS Keychain Secret reference")
	}
	account, err := url.PathUnescape(strings.TrimPrefix(parsed.Path, "/"))
	if err != nil || account == "" || strings.Contains(account, "/") {
		return "", fmt.Errorf("invalid macOS Keychain Secret reference")
	}
	return account, nil
}
