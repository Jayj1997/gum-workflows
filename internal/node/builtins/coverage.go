package builtins

import (
	"embed"
	"fmt"
	"math"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
)

const (
	coverageDefinition       = "go-coverage-check"
	coverageVersion          = "v1"
	coverageAdapterID        = "go-coverage-check/v1"
	coverageBundleDigest     = "sha256:13a48ecd22500bd42fb00569fbaef24976c5a82e1767a1e5d38744816ea82e37"
	defaultCoverageThreshold = 80.0
)

//go:embed scripts/go-coverage-check/v1/manifest.yaml
//go:embed scripts/go-coverage-check/v1/check.sh
var coverageFiles embed.FS

type coverageExecutor struct{}

func (coverageExecutor) Definition() string { return coverageDefinition }
func (coverageExecutor) Version() string    { return coverageVersion }

func (coverageExecutor) Create(config node.Config) (node.Node, error) {
	threshold, err := coverageThreshold(config)
	if err != nil {
		return nil, fmt.Errorf("go-coverage-check config: %w", err)
	}
	bundle, err := loadCoverageBundle()
	if err != nil {
		return nil, err
	}
	return scriptnode.New(bundle, coverageDefinition, coverageVersion, coverageAdapterID,
		func(record scriptnode.ExecutionRecord) (artifact.Artifact, error) {
			result, err := scriptnode.AdaptCoverageResult(record, threshold)
			if err != nil {
				return artifact.Artifact{}, err
			}
			return artifact.Artifact{ID: "go-coverage-check-result", Kind: artifact.KindQualityCheckResult, Data: result}, nil
		})
}

func (coverageExecutor) ValidateExecutor() error {
	bundle, err := loadCoverageBundle()
	if err != nil {
		return err
	}
	return bundle.Validate(coverageDefinition, coverageVersion)
}

func (coverageExecutor) ValidateHostRequirements() error {
	bundle, err := loadCoverageBundle()
	if err != nil {
		return err
	}
	return bundle.ValidateHostRequirements()
}

func coverageThreshold(config node.Config) (float64, error) {
	for field := range config {
		if field != "minimumStatementCoverage" {
			return 0, fmt.Errorf("field %q is not supported", field)
		}
	}
	value, configured := config["minimumStatementCoverage"]
	if !configured {
		return defaultCoverageThreshold, nil
	}
	var threshold float64
	switch number := value.(type) {
	case int:
		threshold = float64(number)
	case int8:
		threshold = float64(number)
	case int16:
		threshold = float64(number)
	case int32:
		threshold = float64(number)
	case int64:
		threshold = float64(number)
	case uint:
		threshold = float64(number)
	case uint8:
		threshold = float64(number)
	case uint16:
		threshold = float64(number)
	case uint32:
		threshold = float64(number)
	case uint64:
		threshold = float64(number)
	case float32:
		threshold = float64(number)
	case float64:
		threshold = number
	default:
		return 0, fmt.Errorf("minimumStatementCoverage must be a number between 0 and 100")
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 100 {
		return 0, fmt.Errorf("minimumStatementCoverage must be a number between 0 and 100")
	}
	return threshold, nil
}

func loadCoverageBundle() (scriptnode.Bundle, error) {
	manifestBytes, err := coverageFiles.ReadFile("scripts/go-coverage-check/v1/manifest.yaml")
	if err != nil {
		return scriptnode.Bundle{}, fmt.Errorf("load go-coverage-check manifest: %w", err)
	}
	manifest, err := scriptnode.LoadManifest(manifestBytes)
	if err != nil {
		return scriptnode.Bundle{}, err
	}
	script, err := coverageFiles.ReadFile("scripts/go-coverage-check/v1/check.sh")
	if err != nil {
		return scriptnode.Bundle{}, fmt.Errorf("load go-coverage-check entry: %w", err)
	}
	return scriptnode.Bundle{
		Manifest: manifest, ManifestBytes: manifestBytes,
		Files: map[string][]byte{"check.sh": script}, ExpectedDigest: coverageBundleDigest,
	}, nil
}
