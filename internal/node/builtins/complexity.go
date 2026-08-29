package builtins

import (
	"embed"
	"fmt"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
)

const (
	complexityDefinition               = "go-complexity-check"
	complexityVersion                  = "v1"
	complexityAdapterID                = "go-complexity-check/v1"
	complexityBundleDigest             = "sha256:5d86984e1250854d2ccfbb66376083747168f13efdf1457853bc4d88345b1734"
	defaultMaximumCyclomaticComplexity = 15
)

//go:embed scripts/go-complexity-check/v1/manifest.yaml
//go:embed scripts/go-complexity-check/v1/check.sh
//go:embed scripts/go-complexity-check/v1/analyzer.go
var complexityFiles embed.FS

type complexityExecutor struct{}

func (complexityExecutor) Definition() string { return complexityDefinition }
func (complexityExecutor) Version() string    { return complexityVersion }

func (complexityExecutor) Create(config node.Config) (node.Node, error) {
	policy, err := complexityPolicy(config)
	if err != nil {
		return nil, fmt.Errorf("go-complexity-check config: %w", err)
	}
	bundle, err := loadComplexityBundle()
	if err != nil {
		return nil, err
	}
	return scriptnode.New(bundle, complexityDefinition, complexityVersion, complexityAdapterID, func(record scriptnode.ExecutionRecord) (artifact.Artifact, error) {
		result, err := scriptnode.AdaptComplexityResult(record, policy)
		if err != nil {
			return artifact.Artifact{}, err
		}
		return artifact.Artifact{ID: "go-complexity-check-result", Kind: artifact.KindQualityCheckResult, Data: result}, nil
	})
}

func (complexityExecutor) ValidateExecutor() error {
	bundle, err := loadComplexityBundle()
	if err != nil {
		return err
	}
	return bundle.Validate(complexityDefinition, complexityVersion)
}

func (complexityExecutor) ValidateHostRequirements() error {
	bundle, err := loadComplexityBundle()
	if err != nil {
		return err
	}
	return bundle.ValidateHostRequirements()
}

func complexityPolicy(config node.Config) (scriptnode.ComplexityPolicy, error) {
	policy := scriptnode.ComplexityPolicy{MaximumCyclomaticComplexity: defaultMaximumCyclomaticComplexity, ExcludeGeneratedFiles: true}
	for field, value := range config {
		switch field {
		case "maximumCyclomaticComplexity":
			maximum, ok := value.(int)
			if !ok || maximum < 1 {
				return scriptnode.ComplexityPolicy{}, fmt.Errorf("maximumCyclomaticComplexity must be a positive integer")
			}
			policy.MaximumCyclomaticComplexity = maximum
		case "includeTests":
			include, ok := value.(bool)
			if !ok {
				return scriptnode.ComplexityPolicy{}, fmt.Errorf("includeTests must be a boolean")
			}
			policy.IncludeTests = include
		case "excludeGeneratedFiles":
			exclude, ok := value.(bool)
			if !ok {
				return scriptnode.ComplexityPolicy{}, fmt.Errorf("excludeGeneratedFiles must be a boolean")
			}
			policy.ExcludeGeneratedFiles = exclude
		default:
			return scriptnode.ComplexityPolicy{}, fmt.Errorf("field %q is not supported", field)
		}
	}
	return policy, nil
}

func loadComplexityBundle() (scriptnode.Bundle, error) {
	manifestBytes, err := complexityFiles.ReadFile("scripts/go-complexity-check/v1/manifest.yaml")
	if err != nil {
		return scriptnode.Bundle{}, fmt.Errorf("load go-complexity-check manifest: %w", err)
	}
	manifest, err := scriptnode.LoadManifest(manifestBytes)
	if err != nil {
		return scriptnode.Bundle{}, err
	}
	files := map[string][]byte{}
	for _, name := range []string{"check.sh", "analyzer.go"} {
		data, readErr := complexityFiles.ReadFile("scripts/go-complexity-check/v1/" + name)
		if readErr != nil {
			return scriptnode.Bundle{}, fmt.Errorf("load go-complexity-check file %s: %w", name, readErr)
		}
		files[name] = data
	}
	return scriptnode.Bundle{Manifest: manifest, ManifestBytes: manifestBytes, Files: files, ExpectedDigest: complexityBundleDigest}, nil
}
