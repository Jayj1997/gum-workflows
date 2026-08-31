package product_test

import (
	"context"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/product"
)

func TestFakeApplicationOpensProductWorkspace(t *testing.T) {
	t.Parallel()

	view, err := product.NewFakeApplication().OpenWorkspace(context.Background())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	if view.Title != "Gum Workflows" {
		t.Fatalf("title = %q, want Gum Workflows", view.Title)
	}
	if view.Message != "Product application round-trip complete" {
		t.Fatalf("message = %q", view.Message)
	}
}
