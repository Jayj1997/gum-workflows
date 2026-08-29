package scriptnode

import (
	"errors"
	"io"
	"sync"
)

var errLogLimit = errors.New("log output exceeded fixed limit")

type logBudget struct {
	mu        sync.Mutex
	remaining int64
	exceeded  bool
	failure   error
	onLimit   func()
}

func newLogBudget(limit int64, onLimit func()) *logBudget {
	return &logBudget{remaining: limit, onLimit: onLimit}
}

func (b *logBudget) writer(destination io.Writer) io.Writer {
	return &budgetWriter{budget: b, destination: destination}
}

func (b *logBudget) err() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.exceeded {
		return errLogLimit
	}
	return b.failure
}

type budgetWriter struct {
	budget      *logBudget
	destination io.Writer
}

func (w *budgetWriter) Write(data []byte) (int, error) {
	w.budget.mu.Lock()
	if w.budget.failure != nil {
		failure := w.budget.failure
		w.budget.mu.Unlock()
		return 0, failure
	}
	if w.budget.exceeded {
		w.budget.mu.Unlock()
		return 0, errLogLimit
	}
	allowed := int64(len(data))
	if allowed > w.budget.remaining {
		allowed = w.budget.remaining
	}
	written, writeErr := w.destination.Write(data[:allowed])
	w.budget.remaining -= int64(written)
	exceeded := int64(len(data)) > allowed
	if exceeded {
		w.budget.exceeded = true
	}
	if writeErr == nil && written != int(allowed) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		w.budget.failure = writeErr
	}
	onLimit := w.budget.onLimit
	w.budget.mu.Unlock()

	if writeErr != nil {
		onLimit()
		return written, writeErr
	}
	if exceeded {
		onLimit()
		return written, errLogLimit
	}
	return written, nil
}
