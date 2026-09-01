//go:build darwin

package secret_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/secret"
)

type keychainCall struct {
	service string
	account string
	value   string
}

type keychainBackendStub struct {
	stores  []keychainCall
	deletes []keychainCall
	value   string
	err     error
}

func (s *keychainBackendStub) Store(_ context.Context, service, account, value string) error {
	s.stores = append(s.stores, keychainCall{service: service, account: account, value: value})
	return s.err
}

func (s *keychainBackendStub) Resolve(_ context.Context, _, _ string) (string, error) {
	return s.value, s.err
}

func (s *keychainBackendStub) Delete(_ context.Context, service, account string) error {
	s.deletes = append(s.deletes, keychainCall{service: service, account: account})
	return s.err
}

func TestKeychainAdapterStoresResolvesAndDeletesThroughNativeBoundary(t *testing.T) {
	ctx := context.Background()
	backend := &keychainBackendStub{}
	adapter := secret.NewKeychainAdapter(backend)

	reference, err := adapter.Store(ctx, "llm-provider/provider-id", "sk-super-secret")
	if err != nil {
		t.Fatalf("store Keychain Secret: %v", err)
	}
	if reference != "keychain://com.gum-workflows.llm-provider/provider-id" {
		t.Fatalf("reference = %q", reference)
	}
	wantStore := keychainCall{service: "com.gum-workflows.llm-provider", account: "provider-id", value: "sk-super-secret"}
	if len(backend.stores) != 1 || backend.stores[0] != wantStore {
		t.Fatalf("store call = %#v, want %#v", backend.stores, wantStore)
	}

	backend.value = "sk-super-secret"
	value, err := adapter.Resolve(ctx, reference)
	if err != nil || value != "sk-super-secret" {
		t.Fatalf("resolve Keychain Secret = %q, %v", value, err)
	}
	if err := adapter.Delete(ctx, reference); err != nil {
		t.Fatalf("delete Keychain Secret: %v", err)
	}
	wantDelete := keychainCall{service: "com.gum-workflows.llm-provider", account: "provider-id"}
	if len(backend.deletes) != 1 || backend.deletes[0] != wantDelete {
		t.Fatalf("delete call = %#v, want %#v", backend.deletes, wantDelete)
	}
}

func TestKeychainAdapterReturnsSanitizedUnavailableError(t *testing.T) {
	backend := &keychainBackendStub{err: errors.New("backend failed with sk-must-not-leak")}
	adapter := secret.NewKeychainAdapter(backend)
	_, err := adapter.Store(context.Background(), "llm-provider/provider-id", "sk-must-not-leak")
	if err == nil || err.Error() != "store secret: macOS Keychain unavailable" {
		t.Fatalf("error = %v", err)
	}
	if !errors.Is(err, backend.err) {
		t.Fatalf("error chain does not preserve backend failure: %v", err)
	}
}
