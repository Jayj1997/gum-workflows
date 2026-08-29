package scriptnode

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/artifact"
	"github.com/Jayj1997/gum-workflows/internal/node"
)

const maxLogBytes int64 = 32 << 20

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
	bundle          Bundle
	nodeName        string
	executorVersion string
	adapterID       string
	adapter         ResultAdapter
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
	return &Node{
		bundle: bundle, nodeName: nodeName, executorVersion: executorVersion,
		adapterID: adapterID, adapter: adapter,
	}, nil
}

// Execute runs the POSIX entry with workspace and tool-output as fixed arguments.
func (n *Node) Execute(ctx node.ExecutionContext, inputs map[string]artifact.ArtifactRef) (map[string]artifact.ArtifactRef, error) {
	if err := n.validateBundle(); err != nil {
		return nil, err
	}
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
	if err := validateRunPaths(ctx.Project.Workspace, ctx.Run.LogsDir, ctx.Run.ToolOutputDir); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: %w", err))
	}
	if filepath.Clean(code.URI) != filepath.Clean(ctx.Project.Workspace) {
		return nil, node.Structural(fmt.Errorf("script node: code reference %q does not identify Project Workspace %q", code.URI, ctx.Project.Workspace))
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
	if ctx.Diagnostics != nil {
		*ctx.Diagnostics = node.RunDiagnostics{
			BundleDigest: n.bundle.ExpectedDigest,
			Host:         map[string]string{"goos": runtime.GOOS, "goarch": runtime.GOARCH},
			Launcher:     launcher, Executables: executables,
			ResultAdapter: n.bundle.Manifest.ResultAdapter,
		}
	}
	toolchain, err := diagnoseImportantTools(ctx.Context, executables)
	if err != nil {
		return nil, node.Structural(fmt.Errorf("script node: diagnose host tools: %w", err))
	}
	if ctx.Diagnostics != nil {
		ctx.Diagnostics.Toolchain = toolchain
	}

	if err := os.MkdirAll(ctx.Run.LogsDir, 0o755); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: create logs directory: %w", err))
	}
	if err := os.RemoveAll(ctx.Run.ToolOutputDir); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: clear tool-output directory: %w", err))
	}
	if err := os.MkdirAll(ctx.Run.ToolOutputDir, 0o755); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: create tool-output directory: %w", err))
	}
	toolOutputCleaned := false
	defer func() {
		if !toolOutputCleaned {
			_ = os.RemoveAll(ctx.Run.ToolOutputDir)
		}
	}()
	bundleDir := filepath.Join(filepath.Dir(ctx.Run.ToolOutputDir), "bundle")
	entryPath, err := n.materialize(bundleDir)
	if err != nil {
		return nil, node.Structural(err)
	}
	if err := n.validateMaterializedBundle(bundleDir); err != nil {
		return nil, err
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
		ctx.Diagnostics.CWD = ctx.Project.Workspace
		ctx.Diagnostics.Arguments = arguments
		ctx.Diagnostics.Logs = map[string]artifact.ArtifactRef{
			"stdout": {ID: "stdout", Kind: artifact.KindLog, URI: stdoutPath},
			"stderr": {ID: "stderr", Kind: artifact.KindLog, URI: stderrPath},
		}
	}

	started := time.Now().UTC()
	command := exec.CommandContext(ctx.Context, launcher, entryPath, arguments[0], arguments[1])
	command.Dir = ctx.Project.Workspace
	configureProcessGroup(command)
	budget := newLogBudget(maxLogBytes, func() { _ = terminateProcessGroup(command) })
	command.Stdout = budget.writer(stdout)
	command.Stderr = budget.writer(stderr)
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
	if budget.isExceeded() {
		return nil, node.Structural(fmt.Errorf("script node: %w (%d bytes)", errLogLimit, maxLogBytes))
	}
	if err := n.validateBundle(); err != nil {
		return nil, err
	}
	if err := n.validateMaterializedBundle(bundleDir); err != nil {
		return nil, err
	}
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			return nil, node.Structural(fmt.Errorf("script node: launch: %w", runErr))
		}
		exitCode = exitErr.ExitCode()
	}
	if err := n.validateToolOutputs(ctx.Run.ToolOutputDir); err != nil {
		return nil, err
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
	if err := n.validateToolOutputs(ctx.Run.ToolOutputDir); err != nil {
		return nil, err
	}
	if provider, ok := result.Data.(interface{ ToolchainDiagnostics() map[string]string }); ok && ctx.Diagnostics != nil {
		ctx.Diagnostics.Toolchain = provider.ToolchainDiagnostics()
	}
	if err := os.RemoveAll(ctx.Run.ToolOutputDir); err != nil {
		return nil, node.Structural(fmt.Errorf("script node: remove non-persistent tool-output: %w", err))
	}
	toolOutputCleaned = true
	ref, err := ctx.Store.Put(result)
	if err != nil {
		return nil, node.Structural(fmt.Errorf("script node: store result: %w", err))
	}
	return map[string]artifact.ArtifactRef{"result": ref}, nil
}

func diagnoseImportantTools(ctx context.Context, executables map[string]string) (map[string]string, error) {
	goPath, ok := executables["go"]
	if !ok {
		return nil, nil
	}
	version, err := runDiagnosticCommand(ctx, goPath, "version")
	if err != nil {
		return nil, fmt.Errorf("go version: %w", err)
	}
	environment, err := runDiagnosticCommand(ctx, goPath, "env", "GOVERSION", "GOROOT", "GOOS", "GOARCH", "CGO_ENABLED")
	if err != nil {
		return nil, fmt.Errorf("go env: %w", err)
	}
	lines := strings.Split(strings.TrimSuffix(environment, "\n"), "\n")
	if len(lines) != 5 {
		return nil, fmt.Errorf("go env returned %d fields, want 5", len(lines))
	}
	return map[string]string{
		"launcherVersion": strings.TrimSpace(version), "finalVersion": strings.TrimSpace(lines[0]),
		"goroot": strings.TrimSpace(lines[1]), "goos": strings.TrimSpace(lines[2]),
		"goarch": strings.TrimSpace(lines[3]), "cgoEnabled": strings.TrimSpace(lines[4]),
	}, nil
}

func runDiagnosticCommand(ctx context.Context, executable string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	configureProcessGroup(command)
	var stdout, stderr bytes.Buffer
	budget := newLogBudget(64<<10, func() { _ = terminateProcessGroup(command) })
	command.Stdout = budget.writer(&stdout)
	command.Stderr = budget.writer(&stderr)
	err := command.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if budget.isExceeded() {
		return "", fmt.Errorf("diagnostic output exceeded fixed limit")
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func validateRunPaths(workspace, logsDir, toolOutputDir string) error {
	workspace = filepath.Clean(workspace)
	logsDir = filepath.Clean(logsDir)
	toolOutputDir = filepath.Clean(toolOutputDir)
	if !filepath.IsAbs(workspace) || !filepath.IsAbs(logsDir) || !filepath.IsAbs(toolOutputDir) {
		return fmt.Errorf("workspace, logs directory, and tool-output directory must be absolute")
	}
	if filepath.Base(logsDir) != "logs" || filepath.Base(toolOutputDir) != "tool-output" || filepath.Dir(logsDir) != filepath.Dir(toolOutputDir) {
		return fmt.Errorf("logs and tool-output must be sibling Node Run directories")
	}
	if withinPath(workspace, logsDir) || withinPath(workspace, toolOutputDir) {
		return fmt.Errorf("logs and tool-output must be outside Project Workspace")
	}
	resolvedWorkspace, err := resolveExistingPath(workspace)
	if err != nil {
		return fmt.Errorf("resolve Project Workspace: %w", err)
	}
	resolvedLogs, err := resolveExistingPath(logsDir)
	if err != nil {
		return fmt.Errorf("resolve logs directory: %w", err)
	}
	resolvedToolOutput, err := resolveExistingPath(toolOutputDir)
	if err != nil {
		return fmt.Errorf("resolve tool-output directory: %w", err)
	}
	if withinPath(resolvedWorkspace, resolvedLogs) || withinPath(resolvedWorkspace, resolvedToolOutput) {
		return fmt.Errorf("logs and tool-output must resolve outside Project Workspace")
	}
	return nil
}

func resolveExistingPath(value string) (string, error) {
	current := filepath.Clean(value)
	var suffix []string
	for {
		_, err := os.Lstat(current)
		if err == nil {
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	return resolved, nil
}

func withinPath(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func (n *Node) validateToolOutputs(dir string) error {
	declared := make(map[string]ToolOutput, len(n.bundle.Manifest.ToolOutputs))
	allowedDirs := map[string]bool{".": true}
	for _, output := range n.bundle.Manifest.ToolOutputs {
		declared[filepath.FromSlash(output.Path)] = output
		for parent := filepath.Dir(filepath.FromSlash(output.Path)); parent != "."; parent = filepath.Dir(parent) {
			allowedDirs[parent] = true
		}
	}
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("tool output %q must not be a symbolic link", filepath.ToSlash(relative))
		}
		if entry.IsDir() {
			if !allowedDirs[relative] {
				return fmt.Errorf("undeclared tool output directory %q", filepath.ToSlash(relative))
			}
			return nil
		}
		if _, ok := declared[relative]; !ok {
			return fmt.Errorf("undeclared tool output %q", filepath.ToSlash(relative))
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("tool output %q must be a regular file", filepath.ToSlash(relative))
		}
		return nil
	}); err != nil {
		return node.Structural(fmt.Errorf("script node: validate tool-output directory: %w", err))
	}
	for relative, output := range declared {
		if !output.Required {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dir, relative)); err != nil {
			return node.Structural(fmt.Errorf("script node: required tool output %q: %w", output.Path, err))
		}
	}
	return nil
}

func (n *Node) validateBundle() error {
	if err := n.bundle.Validate(n.nodeName, n.executorVersion); err != nil {
		return node.Structural(fmt.Errorf("script node: validate immutable bundle: %w", err))
	}
	if n.bundle.Manifest.ResultAdapter != n.adapterID {
		return node.Structural(fmt.Errorf("script node: manifest result adapter %q does not match %q", n.bundle.Manifest.ResultAdapter, n.adapterID))
	}
	return nil
}

func (n *Node) validateMaterializedBundle(dir string) error {
	expected := n.bundle.Files
	allowedDirs := map[string]bool{".": true}
	for name := range expected {
		for parent := filepath.Dir(filepath.FromSlash(name)); parent != "."; parent = filepath.Dir(parent) {
			allowedDirs[parent] = true
		}
	}
	seen := make(map[string]bool, len(expected))
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("materialized bundle file %q must not be a symbolic link", filepath.ToSlash(relative))
		}
		if entry.IsDir() {
			if !allowedDirs[relative] {
				return fmt.Errorf("undeclared materialized bundle directory %q", filepath.ToSlash(relative))
			}
			return nil
		}
		name := filepath.ToSlash(relative)
		want, ok := expected[name]
		if !ok {
			return fmt.Errorf("undeclared materialized bundle file %q", name)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, want) {
			return fmt.Errorf("materialized bundle file %q does not match immutable digest", name)
		}
		seen[name] = true
		return nil
	}); err != nil {
		return node.Structural(fmt.Errorf("script node: validate materialized bundle: %w", err))
	}
	for name := range expected {
		if !seen[name] {
			return node.Structural(fmt.Errorf("script node: validate materialized bundle: file %q is missing", name))
		}
	}
	return nil
}

func (n *Node) materialize(dir string) (string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("script node: clear bundle directory: %w", err)
	}
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
