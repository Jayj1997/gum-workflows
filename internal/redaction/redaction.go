// Package redaction removes resolved Secret values and sensitive request
// headers from text that leaves the credential boundary: log lines, error
// messages and diagnostics bundles. Redaction is exact-substring replacement
// of registered canary values plus structural removal of Authorization-style
// headers, so a redacted string never contains the original secret.
package redaction

import (
	"slices"
	"sort"
	"strings"
	"sync"
)

// Placeholder replaces every occurrence of a registered Secret value.
const Placeholder = "[REDACTED]"

// Redactor holds the Secret values active for one process and rewrites text
// so none of them survives. It is safe for concurrent use. Secrets are
// registered when resolved and never persisted.
type Redactor struct {
	mu      sync.RWMutex
	phrases []string // descending length, so longer secrets win
}

// NewRedactor returns an empty Redactor.
func NewRedactor() *Redactor { return &Redactor{} }

// Register adds one Secret value to be removed from all future Redact calls.
// Empty values are ignored; registering the same value twice is a no-op.
func (r *Redactor) Register(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if slices.Contains(r.phrases, value) {
		return
	}
	r.phrases = append(r.phrases, value)
	// Longest first: redacting "sk-long-secret" must not be broken up by a
	// shorter registered secret sharing its prefix.
	sort.Slice(r.phrases, func(i, j int) bool {
		return len(r.phrases[i]) > len(r.phrases[j])
	})
}

// Redact replaces every registered Secret value and every Authorization,
// Proxy-Authorization, Cookie or Set-Cookie header in text. It always returns
// a value that contains none of the registered secrets.
func (r *Redactor) Redact(text string) string {
	r.mu.RLock()
	phrases := make([]string, len(r.phrases))
	copy(phrases, r.phrases)
	r.mu.RUnlock()

	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			text = strings.ReplaceAll(text, phrase, Placeholder)
		}
	}
	return redactHeaders(text)
}

// redactHeaders removes header names and values of sensitive request headers
// whether they appear as "Authorization: Bearer x", "Authorization: x" or as
// URL-encoded wire text. Case-insensitive per RFC 9110.
func redactHeaders(text string) string {
	return sensitiveHeaderPattern.ReplaceAllString(text, "${1}"+Placeholder)
}
