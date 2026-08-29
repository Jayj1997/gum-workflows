package scriptnode

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

// ExecutionRecord is the process and filesystem evidence passed to a Result Adapter.
type ExecutionRecord struct {
	ExitCode      int
	ToolOutputDir string
	StdoutPath    string
	StderrPath    string
	Code          artifact.ArtifactRef
	StartedAt     time.Time
	FinishedAt    time.Time
}

// ResultAdapter turns formal tool evidence into the ScriptNode's sole Artifact body.
type ResultAdapter func(ExecutionRecord) (artifact.Artifact, error)

// Node executes one immutable Automation Script Bundle.
type Node struct {
	bundle  Bundle
	adapter ResultAdapter
}

// New validates and constructs a ScriptNode for one fixed executor identity.
func New(bundle Bundle, nodeName, executorVersion, adapterID string, adapter ResultAdapter) (*Node, error) {
	if err := bundle.Validate(nodeName, executorVersion); err != nil {
		return nil, fmt.Errorf("create script node: %w", err)
	}
	if bundle.Manifest.ResultAdapter != adapterID {
		return nil, fmt.Errorf("create script node: manifest result adapter %q does not match %q", bundle.Manifest.ResultAdapter, adapterID)
	}
	if adapter == nil {
		return nil, fmt.Errorf("create script node: result adapter must not be nil")
	}
	return &Node{bundle: bundle, adapter: adapter}, nil
}

// Execute runs the POSIX entry with workspace and tool-output as fixed arguments.
func (n *Node) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	code, ok := inputs["code"]
	if !ok || code.Kind != artifact.KindSourceCode {
		return nil, node.Structural(fmt.Errorf("script node: input code must be a SourceCode reference"))
	}
	if err := code.Validate(); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: input code: %w", err))
	}
	if ctx.Project.Workspace == "" || ctx.Run.LogsDir == "" || ctx.Run.ToolOutputDir == "" {
		return nil, node.Structural(fmt.Errorf("script node: workspace, logs directory, and tool-output directory must not be empty"))
	}
	if !slices.Contains(n.bundle.Manifest.Platforms, runtime.GOOS) {
		return nil, node.Structural(fmt.Errorf("script node: platform %q is not supported", runtime.GOOS))
	}

	executables := make(map[string]string, len(n.bundle.Manifest.Requirements.Executables))
	for _, name := range n.bundle.Manifest.Requirements.Executables {
		resolved, err := exec.LookPath(name)
		if err != nil {
			return nil, node.Structural(fmt.Errorf("script node: required executable %q: %w", name, err))
		}
		executables[name] = resolved
	}
	launcher, ok := executables["sh"]
	if !ok {
		var err error
		launcher, err = exec.LookPath("sh")
		if err != nil {
			return nil, node.Structural(fmt.Errorf("script node: POSIX shell: %w", err))
		}
	}

	if err := os.MkdirAll(ctx.Run.LogsDir, 0o755); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: create logs directory: %w", err))
	}
	if err := os.MkdirAll(ctx.Run.ToolOutputDir, 0o755); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: create tool-output directory: %w", err))
	}
	bundleDir := filepath.Join(filepath.Dir(ctx.Run.ToolOutputDir), "bundle")
	entryPath, err := n.materialize(bundleDir)
	if err != nil {
		return nil, node.Structural(err)
	}

	stdoutPath := filepath.Join(ctx.Run.LogsDir, "stdout.log")
	stderrPath := filepath.Join(ctx.Run.LogsDir, "stderr.log")
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, node.Structural(fmt.Errorf("script node: open stdout log: %w", err))
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, node.Structural(fmt.Errorf("script node: open stderr log: %w", err))
	}
	defer stderr.Close()

	arguments := []string{ctx.Project.Workspace, ctx.Run.ToolOutputDir}
	if ctx.Diagnostics != nil {
		*ctx.Diagnostics = node.RunDiagnostics{
			BundleDigest: n.bundle.ExpectedDigest, CWD: ctx.Project.Workspace,
			Arguments: arguments, Launcher: launcher, Executables: executables,
			ResultAdapter: n.bundle.Manifest.ResultAdapter,
			Logs: map[string]artifact.ArtifactRef{
				"stdout": {ID: "stdout", Kind: artifact.KindLog, URI: stdoutPath},
				"stderr": {ID: "stderr", Kind: artifact.KindLog, URI: stderrPath},
			},
		}
	}

	started := time.Now().UTC()
	command := exec.CommandContext(ctx.Context, launcher, entryPath, arguments[0], arguments[1])
	command.Dir = ctx.Project.Workspace
	command.Stdout = stdout
	command.Stderr = stderr
	runErr := command.Run()
	finished := time.Now().UTC()
	if closeErr := stdout.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("close stdout log: %w", closeErr)
	}
	if closeErr := stderr.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("close stderr log: %w", closeErr)
	}
	if ctx.Err() != nil {
		return nil, node.Structural(fmt.Errorf("script node: %w", ctx.Err()))
	}
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, node.Structural(fmt.Errorf("script node: launch: %w", runErr))
		}
		exitCode = exitErr.ExitCode()
	}
	for _, output := range n.bundle.Manifest.ToolOutputs {
		if !output.Required {
			continue
		}
		info, statErr := os.Stat(filepath.Join(ctx.Run.ToolOutputDir, filepath.FromSlash(output.Path)))
		if statErr != nil || !info.Mode().IsRegular() {
			if statErr == nil {
				statErr = fmt.Errorf("not a regular file")
			}
			return nil, node.Structural(fmt.Errorf("script node: required tool output %q: %w", output.Path, statErr))
		}
	}

	result, err := n.adapter(ExecutionRecord{
		ExitCode: exitCode, ToolOutputDir: ctx.Run.ToolOutputDir,
		StdoutPath: stdoutPath, StderrPath: stderrPath, Code: code,
		StartedAt: started, FinishedAt: finished,
	})
	if err != nil {
		return nil, node.Structural(fmt.Errorf("script node: adapt result: %w", err))
	}
	if result.Kind != artifact.KindQualityCheckResult {
		return nil, node.Structural(fmt.Errorf("script node: result adapter returned kind %q, want %q", result.Kind, artifact.KindQualityCheckResult))
	}
	ref, err := ctx.Store.Put(result)
	if err != nil {
		return nil, node.Structural(fmt.Errorf("script node: store result: %w", err))
	}
	return map[string]artifact.ArtifactRef{"result": ref}, nil
}

func (n *Node) materialize(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("script node: create bundle directory: %w", err)
	}
	for name, data := range n.bundle.Files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("script node: create bundle path: %w", err)
		}
		mode := os.FileMode(0o444)
		if name == n.bundle.Manifest.Entry {
			mode = 0o555
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			return "", fmt.Errorf("script node: write bundle file %q: %w", name, err)
		}
	}
	return filepath.Join(dir, filepath.FromSlash(n.bundle.Manifest.Entry)), nil
}

func runtimePlatform() string { return runtime.GOOS }
