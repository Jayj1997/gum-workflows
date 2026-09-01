//go:build darwin && !cgo

package secret

import (
	"context"
	"fmt"
)

type unavailableKeychainBackend struct{}

func newSecurityFrameworkBackend() KeychainBackend {
	return unavailableKeychainBackend{}
}

func (unavailableKeychainBackend) Store(context.Context, string, string, string) error {
	return fmt.Errorf("macOS Keychain requires cgo")
}

func (unavailableKeychainBackend) Resolve(context.Context, string, string) (string, error) {
	return "", fmt.Errorf("macOS Keychain requires cgo")
}

func (unavailableKeychainBackend) Delete(context.Context, string, string) error {
	return fmt.Errorf("macOS Keychain requires cgo")
}
