package execution

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextExecutionID(t *testing.T) {
	t.Run("empty base dir", func(t *testing.T) {
		id, err := NextExecutionID(filepath.Join(t.TempDir(), "nonexistent"))
		if err != nil {
			t.Fatalf("NextExecutionID() unexpected error: %v", err)
		}
		if id != "execution-000001" {
			t.Errorf("id = %q, want execution-000001", id)
		}
	})

	t.Run("continues from max", func(t *testing.T) {
		base := t.TempDir()
		for _, name := range []string{"execution-000001", "execution-000002", "execution-000007"} {
			if err := os.MkdirAll(filepath.Join(base, name), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		// 无关目录与文件不参与编号。
		if err := os.MkdirAll(filepath.Join(base, "workspace"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(base, "execution-bogus"), nil, 0o644); err != nil {
			t.Fatal(err)
		}

		id, err := NextExecutionID(base)
		if err != nil {
			t.Fatalf("NextExecutionID() unexpected error: %v", err)
		}
		if id != "execution-000008" {
			t.Errorf("id = %q, want execution-000008", id)
		}
	})

	t.Run("rejects unreadable dir", func(t *testing.T) {
		// 用一个文件路径充当目录：ReadDir 报错（非 IsNotExist）。
		file := filepath.Join(t.TempDir(), "afile")
		if err := os.WriteFile(file, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := NextExecutionID(file); err == nil {
			t.Error("NextExecutionID(file) = nil error, want failure")
		}
	})
}
