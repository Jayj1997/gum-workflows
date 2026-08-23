package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"validate"}, {"run", "x.yaml"}, {"validate", "a", "b"}} {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) = nil error, want usage error", args)
		}
	}
	if err := run([]string{"bogus", "x.yaml"}); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run(bogus) error = %v, want unknown command", err)
	}
}

func TestValidateCmd(t *testing.T) {
	t.Run("valid workflow", func(t *testing.T) {
		err := validateCmd(filepath.Join("..", "..", "internal", "workflow", "testdata", "valid.yaml"))
		if err != nil {
			t.Fatalf("validateCmd() unexpected error: %v", err)
		}
	})

	t.Run("example workflow passes full pipeline", func(t *testing.T) {
		if err := validateCmd(filepath.Join("..", "..", "examples", "fullstack", "workflow.yaml")); err != nil {
			t.Fatalf("validateCmd(example) unexpected error: %v", err)
		}
	})

	t.Run("semantic violation is rejected", func(t *testing.T) {
		// unknown-type 结构合法（CUE 通过），语义层必须拦截。
		path := filepath.Join("..", "..", "internal", "validation", "testdata", "invalid-node", "unknown-type.yaml")
		err := validateCmd(path)
		if err == nil {
			t.Fatal("validateCmd(unknown-type) = nil error, want semantic rejection")
		}
		if !strings.Contains(err.Error(), "unknown node type") {
			t.Errorf("error %q should mention unknown node type", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if err := validateCmd("nonexistent.yaml"); err == nil {
			t.Fatal("validateCmd() = nil error, want read failure")
		}
	})
}
