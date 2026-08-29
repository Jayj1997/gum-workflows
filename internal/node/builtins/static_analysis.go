package builtins

import (
	"embed"
	"fmt"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
)

const (
	staticAnalysisDefinition   = "go-static-analysis"
	staticAnalysisVersion      = "v1"
	staticAnalysisAdapterID    = "go-static-analysis/v1"
	staticAnalysisBundleDigest = "sha256:722692dec1571a3f9fc2aa547ac82b13f42cf13db72a884722628f401418ac38"
)

//go:embed scripts/go-static-analysis/v1/manifest.yaml
//go:embed scripts/go-static-analysis/v1/check.sh
var staticAnalysisFiles embed.FS

type staticAnalysisExecutor struct{}

func (staticAnalysisExecutor) Definition() string { return staticAnalysisDefinition }
func (staticAnalysisExecutor) Version() string    { return staticAnalysisVersion }

func (staticAnalysisExecutor) Create(config node.Config) (node.Node, error) {
	if len(config) != 0 {
		return nil, fmt.Errorf("go-static-analysis config: no fields are supported")
	}
	bundle, err := loadStaticAnalysisBundle()
	if err != nil {
		return nil, err
	}
	return scriptnode.New(bundle, staticAnalysisDefinition, staticAnalysisVersion, staticAnalysisAdapterID, adaptStaticAnalysis)
}

func (staticAnalysisExecutor) ValidateExecutor() error {
	bundle, err := loadStaticAnalysisBundle()
	if err != nil {
		return err
	}
	return bundle.Validate(staticAnalysisDefinition, staticAnalysisVersion)
}

func (staticAnalysisExecutor) ValidateHostRequirements() error {
	bundle, err := loadStaticAnalysisBundle()
	if err != nil {
		return err
	}
	return bundle.ValidateHostRequirements()
}

func loadStaticAnalysisBundle() (scriptnode.Bundle, error) {
	manifestBytes, err := staticAnalysisFiles.ReadFile("scripts/go-static-analysis/v1/manifest.yaml")
	if err != nil {
		return scriptnode.Bundle{}, fmt.Errorf("load go-static-analysis manifest: %w", err)
	}
	manifest, err := scriptnode.LoadManifest(manifestBytes)
	if err != nil {
		return scriptnode.Bundle{}, err
	}
	script, err := staticAnalysisFiles.ReadFile("scripts/go-static-analysis/v1/check.sh")
	if err != nil {
		return scriptnode.Bundle{}, fmt.Errorf("load go-static-analysis entry: %w", err)
	}
	bundle := scriptnode.Bundle{
		Manifest: manifest, ManifestBytes: manifestBytes,
		Files: map[string][]byte{"check.sh": script},
	}
	// Changing these bytes requires a new Executor Version. The pinned digest
	// rejects accidental in-place edits to the immutable v1 asset.
	bundle.ExpectedDigest = staticAnalysisBundleDigest
	return bundle, nil
}

func adaptStaticAnalysis(record scriptnode.ExecutionRecord) (artifact.Artifact, error) {
	result, err := scriptnode.AdaptStaticResult(record)
	if err != nil {
		return artifact.Artifact{}, err
	}
	return artifact.Artifact{
		ID: "go-static-analysis-result", Kind: artifact.KindQualityCheckResult, Data: result,
	}, nil
}
