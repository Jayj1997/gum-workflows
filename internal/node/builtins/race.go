package builtins

import (
	"context"
	"embed"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
	"github.com/Jayj1997/gum-workflows/internal/node/scriptnode"
)

const (
	raceDefinition   = "go-race-check"
	raceVersion      = "v1"
	raceAdapterID    = "go-race-check/v1"
	raceBundleDigest = "sha256:909637fb60f23b6e691da79e610edd5a6ef57c09a3b5815a2e3804b5bcac3c96"
)

//go:embed scripts/go-race-check/v1/manifest.yaml
//go:embed scripts/go-race-check/v1/check.sh
var raceFiles embed.FS

type raceExecutor struct{}

func (raceExecutor) Definition() string { return raceDefinition }
func (raceExecutor) Version() string    { return raceVersion }

func (raceExecutor) Create(config node.Config) (node.Node, error) {
	if len(config) != 0 {
		return nil, fmt.Errorf("go-race-check config: no fields are supported")
	}
	bundle, err := loadRaceBundle()
	if err != nil {
		return nil, err
	}
	delegate, err := scriptnode.New(bundle, raceDefinition, raceVersion, raceAdapterID,
		func(record scriptnode.ExecutionRecord) (artifact.Artifact, error) {
			result, err := scriptnode.AdaptRaceResult(record)
			if err != nil {
				return artifact.Artifact{}, err
			}
			return artifact.Artifact{ID: "go-race-check-result", Kind: artifact.KindQualityCheckResult, Data: result}, nil
		})
	if err != nil {
		return nil, err
	}
	return raceNode{delegate: delegate}, nil
}

func (raceExecutor) ValidateExecutor() error {
	bundle, err := loadRaceBundle()
	if err != nil {
		return err
	}
	return bundle.Validate(raceDefinition, raceVersion)
}

func (raceExecutor) ValidateHostRequirements() error {
	bundle, err := loadRaceBundle()
	if err != nil {
		return err
	}
	if err := bundle.ValidateHostRequirements(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return validateRaceHostRequirements(ctx)
}

type raceNode struct {
	delegate node.Node
}

func (n raceNode) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	if err := validateRaceHostRequirements(ctx.Context); err != nil {
		return nil, node.Structural(fmt.Errorf("go-race-check host requirements: %w", err))
	}
	return n.delegate.Execute(ctx, inputs)
}

func validateRaceHostRequirements(ctx context.Context) error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("required executable %q: %w", "go", err)
	}
	command := exec.CommandContext(ctx, goPath, "env", "GOOS", "GOARCH", "CGO_ENABLED", "CC")
	output, err := command.Output()
	if err != nil {
		return fmt.Errorf("diagnose Go race requirements: %w", err)
	}
	fields := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(fields) != 4 {
		return fmt.Errorf("go env returned %d race requirement fields, want 4", len(fields))
	}
	goos, goarch, cgoEnabled, compiler := fields[0], fields[1], fields[2], strings.TrimSpace(fields[3])
	if goos != runtime.GOOS {
		return fmt.Errorf("Go target GOOS %q must match host %q", goos, runtime.GOOS)
	}
	if !raceSupported(goos, goarch) {
		return fmt.Errorf("Go race detector does not support %s/%s", goos, goarch)
	}
	if cgoEnabled != "1" {
		return fmt.Errorf("Go race detector requires CGO_ENABLED=1 (got %q)", cgoEnabled)
	}
	compilerCommand := strings.Fields(compiler)
	if len(compilerCommand) == 0 {
		return fmt.Errorf("Go race detector requires a configured C compiler")
	}
	if _, err := exec.LookPath(compilerCommand[0]); err != nil {
		return fmt.Errorf("Go race detector C compiler %q: %w", compilerCommand[0], err)
	}
	return nil
}

func raceSupported(goos, goarch string) bool {
	switch goos {
	case "darwin":
		return goarch == "amd64" || goarch == "arm64"
	case "linux":
		switch goarch {
		case "amd64", "arm64", "ppc64le", "s390x", "loong64":
			return true
		}
	}
	return false
}

func loadRaceBundle() (scriptnode.Bundle, error) {
	manifestBytes, err := raceFiles.ReadFile("scripts/go-race-check/v1/manifest.yaml")
	if err != nil {
		return scriptnode.Bundle{}, fmt.Errorf("load go-race-check manifest: %w", err)
	}
	manifest, err := scriptnode.LoadManifest(manifestBytes)
	if err != nil {
		return scriptnode.Bundle{}, err
	}
	script, err := raceFiles.ReadFile("scripts/go-race-check/v1/check.sh")
	if err != nil {
		return scriptnode.Bundle{}, fmt.Errorf("load go-race-check entry: %w", err)
	}
	return scriptnode.Bundle{
		Manifest: manifest, ManifestBytes: manifestBytes,
		Files: map[string][]byte{"check.sh": script}, ExpectedDigest: raceBundleDigest,
	}, nil
}
