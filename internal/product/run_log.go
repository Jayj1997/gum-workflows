package product

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/redaction"
)

// appVersion identifies the running Application build in diagnostics
// bundles. Ticket 16 owns the real build pipeline; until then every build
// reports the same development version.
const appVersion = "0.0.0-dev"

// logSchemaVersion names the JSON log-line envelope written for each Run.
const logSchemaVersion = "productRunLog/v1"

// runLogger writes sanitized, structured Run logs below
// runs/<run-id>/logs/run.log. Every line carries the Workflow Run and Node
// Run identity so a crash can be attributed to one invocation, and every
// message passes the shared Redactor before it reaches disk.
type runLogger struct {
	logger   *slog.Logger
	file     *os.File
	redactor *redaction.Redactor
}

// openRunLogger creates the Run log file below the Local Data Root. A nil
// redactor still scrubs sensitive headers; Run execution never blocks on
// logging: a failed open returns a discarding logger rather than failing the
// Run, because Run history in SQLite remains the durable record.
func openRunLogger(runLogPath string, redactor *redaction.Redactor) *runLogger {
	if redactor == nil {
		redactor = redaction.NewRedactor()
	}
	if runLogPath == "" {
		return &runLogger{logger: slog.New(slog.DiscardHandler), redactor: redactor}
	}
	if err := os.MkdirAll(filepath.Dir(runLogPath), 0o755); err != nil {
		return &runLogger{logger: slog.New(slog.DiscardHandler), redactor: redactor}
	}
	file, err := os.OpenFile(runLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return &runLogger{logger: slog.New(slog.DiscardHandler), redactor: redactor}
	}
	handler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		// INFO by default: request IDs, latencies and phase transitions are
		// the product-relevant signal; debug chatter stays out of the file.
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Value.Kind() == slog.KindString {
				attr.Value = slog.StringValue(redactor.Redact(attr.Value.String()))
			}
			return attr
		},
	})
	logger := slog.New(handler).With("schema", logSchemaVersion)
	return &runLogger{logger: logger, file: file, redactor: redactor}
}

func (l *runLogger) close() {
	if l.file != nil {
		_ = l.file.Close()
	}
}

// logNodeRunStart records that one Node Run began, including the phase and
// the identities the bundle later references.
func (l *runLogger) logNodeRunStart(ctx context.Context, runID, nodeRunID, nodeID, definition string, startedAt time.Time) {
	l.logger.InfoContext(ctx, "node run started",
		"runId", runID, "nodeRunId", nodeRunID, "nodeId", nodeID,
		"nodeDefinition", definition, "phase", "running", "startedAt", startedAt.Format(time.RFC3339Nano))
}

// logNodeRunFinish records one Node Run terminal phase with latency, the
// Provider request ID of a real model call and the sanitized error.
func (l *runLogger) logNodeRunFinish(ctx context.Context, runID, nodeRunID, nodeID, definition, status string, startedAt, finishedAt time.Time, providerRequestID string, runErr error) {
	attrs := []any{
		"runId", runID, "nodeRunId", nodeRunID, "nodeId", nodeID,
		"nodeDefinition", definition, "phase", status,
		"latencyMs", finishedAt.Sub(startedAt).Milliseconds(),
	}
	if providerRequestID != "" {
		attrs = append(attrs, "providerRequestId", providerRequestID)
	}
	if runErr != nil {
		attrs = append(attrs, "error", l.redactor.Redact(runErr.Error()))
	}
	l.logger.InfoContext(ctx, "node run finished", attrs...)
}

// logRunEvent records one Workflow Run level event with sanitized detail.
func (l *runLogger) logRunEvent(ctx context.Context, runID, event, detail string, at time.Time) {
	attrs := []any{"runId", runID, "event", event, "at", at.Format(time.RFC3339Nano)}
	if detail != "" {
		attrs = append(attrs, "detail", l.redactor.Redact(detail))
	}
	l.logger.InfoContext(ctx, "run event", attrs...)
}

// writeLogCopy copies the run log into the diagnostics bundle, redacting one
// more time as a defense in depth: the source file is already redacted, and
// this guard keeps bundle content safe even if a future writer regresses.
func writeLogCopy(destination io.Writer, source string, redactor *redaction.Redactor) error {
	if redactor == nil {
		redactor = redaction.NewRedactor()
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read run log: %w", err)
	}
	if _, err := destination.Write([]byte(redactor.Redact(string(data)))); err != nil {
		return fmt.Errorf("write run log copy: %w", err)
	}
	return nil
}
