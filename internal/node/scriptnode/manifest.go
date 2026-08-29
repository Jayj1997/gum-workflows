// Package scriptnode executes immutable, manifest-described automation bundles.
package scriptnode

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os/exec"
	"path"
	"runtime"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	manifestAPIVersion = "automationScript/v1"
	manifestKind       = "automationScript"
)

// Manifest declares one immutable automation script bundle.
type Manifest struct {
	APIVersion    string             `yaml:"apiVersion"`
	Kind          string             `yaml:"kind"`
	Node          string             `yaml:"node"`
	Executor      string             `yaml:"executor"`
	Entry         string             `yaml:"entry"`
	Platforms     []string           `yaml:"platforms"`
	Requirements  ScriptRequirements `yaml:"requirements"`
	ToolOutputs   []ToolOutput       `yaml:"toolOutputs"`
	ResultAdapter string             `yaml:"resultAdapter"`
}

// ScriptRequirements declares host executables needed before script launch.
type ScriptRequirements struct {
	Executables []string `yaml:"executables"`
}

// ToolOutput declares one relative path owned by the Node Run tool-output directory.
type ToolOutput struct {
	Path     string `yaml:"path"`
	Required bool   `yaml:"required"`
}

// LoadManifest strictly decodes and validates an automationScript/v1 manifest.
func LoadManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("parse automation script manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Manifest{}, fmt.Errorf("parse automation script manifest: expected a single document")
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("validate automation script manifest: %w", err)
	}
	return manifest, nil
}

// Validate checks the closed v1 manifest contract.
func (m Manifest) Validate() error {
	if m.APIVersion != manifestAPIVersion {
		return fmt.Errorf("apiVersion must be %q", manifestAPIVersion)
	}
	if m.Kind != manifestKind {
		return fmt.Errorf("kind must be %q", manifestKind)
	}
	if strings.TrimSpace(m.Node) == "" || strings.TrimSpace(m.Executor) == "" {
		return fmt.Errorf("node and executor must not be empty")
	}
	if !safeRelativePath(m.Entry) {
		return fmt.Errorf("entry %q must be a safe relative path", m.Entry)
	}
	if len(m.Platforms) == 0 {
		return fmt.Errorf("platforms must not be empty")
	}
	for _, executable := range m.Requirements.Executables {
		if strings.TrimSpace(executable) == "" || path.Base(executable) != executable {
			return fmt.Errorf("required executable %q must be a bare command name", executable)
		}
	}
	seen := map[string]bool{}
	for _, output := range m.ToolOutputs {
		if !safeRelativePath(output.Path) {
			return fmt.Errorf("tool output %q must be a safe relative path", output.Path)
		}
		if seen[output.Path] {
			return fmt.Errorf("tool output %q is duplicated", output.Path)
		}
		seen[output.Path] = true
	}
	if strings.TrimSpace(m.ResultAdapter) == "" {
		return fmt.Errorf("resultAdapter must not be empty")
	}
	return nil
}

func safeRelativePath(value string) bool {
	return value != "" && value == path.Clean(value) && value != "." && !path.IsAbs(value) && !strings.HasPrefix(value, "../")
}

// Bundle is the exact manifest and file content fixed by an Executor Version.
type Bundle struct {
	Manifest       Manifest
	ManifestBytes  []byte
	Files          map[string][]byte
	ExpectedDigest string
}

// Validate checks bundle identity and ensures the declared entry is present.
func (b Bundle) Validate(nodeName, executorVersion string) error {
	if err := b.Manifest.Validate(); err != nil {
		return err
	}
	if b.Manifest.Node != nodeName || b.Manifest.Executor != executorVersion {
		return fmt.Errorf("bundle identity (%s, %s) does not match executor (%s, %s)", b.Manifest.Node, b.Manifest.Executor, nodeName, executorVersion)
	}
	if len(b.ManifestBytes) == 0 {
		return fmt.Errorf("manifest bytes must not be empty")
	}
	if b.ExpectedDigest == "" {
		return fmt.Errorf("expected bundle digest must not be empty")
	}
	if actual := b.Digest(); actual != b.ExpectedDigest {
		return fmt.Errorf("bundle digest mismatch: got %s, want %s", actual, b.ExpectedDigest)
	}
	if _, ok := b.Files[b.Manifest.Entry]; !ok {
		return fmt.Errorf("bundle entry %q is missing", b.Manifest.Entry)
	}
	for name := range b.Files {
		if !safeRelativePath(name) {
			return fmt.Errorf("bundle file %q must be a safe relative path", name)
		}
	}
	return nil
}

// ValidateHostRequirements checks platform and executable availability without running a script.
func (b Bundle) ValidateHostRequirements() error {
	if !slices.Contains(b.Manifest.Platforms, runtime.GOOS) {
		return fmt.Errorf("platform %q is not supported (supported: %s)", runtime.GOOS, strings.Join(b.Manifest.Platforms, ", "))
	}
	for _, executable := range b.Manifest.Requirements.Executables {
		if _, err := exec.LookPath(executable); err != nil {
			return fmt.Errorf("required executable %q: %w", executable, err)
		}
	}
	return nil
}

// Digest returns a deterministic digest of the manifest and every bundle file.
func (b Bundle) Digest() string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("manifest.yaml\x00"))
	_, _ = hash.Write(b.ManifestBytes)
	names := make([]string, 0, len(b.Files))
	for name := range b.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		_, _ = hash.Write([]byte("\x00" + name + "\x00"))
		_, _ = hash.Write(b.Files[name])
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}
