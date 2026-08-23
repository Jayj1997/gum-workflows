package artifact

import (
	"strings"
	"testing"
)

func TestNewRegistryContainsMVPKinds(t *testing.T) {
	r := NewRegistry()

	for _, k := range []Kind{
		KindRequirementSpec,
		KindArchitectureSpec,
		KindOpenAPI,
		KindFrontendSDK,
		KindSourceCode,
		KindTestReport,
		KindApprovalResult,
	} {
		if !r.Has(k) {
			t.Errorf("Has(%s) = false, want true", k)
		}
	}
	if r.Has("FigmaDesign") {
		t.Error(`Has("FigmaDesign") = true, want false`)
	}
	if got := len(r.Kinds()); got != 7 {
		t.Errorf("len(Kinds()) = %d, want 7", got)
	}
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()

	if err := r.Register("FigmaDesign"); err != nil {
		t.Fatalf("Register() unexpected error: %v", err)
	}
	if !r.Has("FigmaDesign") {
		t.Error("Has(FigmaDesign) = false after Register")
	}

	if err := r.Register("FigmaDesign"); err == nil {
		t.Fatal("duplicate Register() = nil error, want rejection")
	}
	if err := r.Register(""); err == nil {
		t.Fatal("Register(empty) = nil error, want rejection")
	}
	if !strings.Contains(func() string { err := r.Register(""); return err.Error() }(), "must not be empty") {
		t.Error("empty Register error should mention must not be empty")
	}
}
