package redaction_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/Jayj1997/gum-workflows/internal/redaction"
)

func TestRedactorReplacesRegisteredSecrets(t *testing.T) {
	t.Parallel()
	redactor := redaction.NewRedactor()
	redactor.Register("gum-secret-canary-8291")

	text := "provider rejected key gum-secret-canary-8291 while calling https://api.example/v1"
	got := redactor.Redact(text)
	if strings.Contains(got, "gum-secret-canary-8291") {
		t.Fatalf("redacted text still contains the secret: %s", got)
	}
	if !strings.Contains(got, "provider rejected key "+redaction.Placeholder) {
		t.Fatalf("redacted text lost the surrounding context: %s", got)
	}
}

func TestRedactorRedactsLongerSecretsBeforeShorterOverlaps(t *testing.T) {
	t.Parallel()
	redactor := redaction.NewRedactor()
	redactor.Register("sk-short")
	redactor.Register("sk-short-with-long-suffix")

	got := redactor.Redact("value sk-short-with-long-suffix and sk-short both appear")
	if strings.Contains(got, "sk-short") {
		t.Fatalf("overlap redaction failed: %s", got)
	}
	if strings.Count(got, redaction.Placeholder) != 2 {
		t.Fatalf("expected two redactions, got: %s", got)
	}
}

func TestRedactorRedactsSensitiveHeadersWithoutSecretRegistration(t *testing.T) {
	t.Parallel()
	redactor := redaction.NewRedactor()
	redactor.Register("sk-known")

	cases := map[string]string{
		"lowercase":   `authorization: bearer sk-unknown-value`,
		"titled":      `Authorization: Bearer sk-unknown-value`,
		"no-space":    `Authorization:sk-unknown-value`,
		"proxy":       `Proxy-Authorization: Basic abc`,
		"cookie":      `Cookie: session=abc`,
		"set-cookie":  `Set-Cookie: session=abc; Path=/`,
		"x-api-key":   `X-API-Key: abc`,
		"next-header": "Authorization: Bearer abc\nContent-Type: application/json",
	}
	for name, text := range cases {
		got := redactor.Redact(text)
		if strings.Contains(got, "sk-unknown-value") && name != "next-header" {
			t.Fatalf("%s: header value survived: %s", name, got)
		}
		if strings.Contains(got, "Bearer abc") || strings.Contains(got, "Basic abc") || strings.Contains(got, "session=abc") {
			t.Fatalf("%s: header value survived: %s", name, got)
		}
	}
	// Non-sensitive headers stay readable for protocol diagnosis.
	got := redactor.Redact("Authorization: Bearer abc\nContent-Type: application/json")
	if !strings.Contains(got, "Content-Type: application/json") {
		t.Fatalf("content-type header was redacted: %s", got)
	}
}

func TestRedactorIgnoresEmptyAndDuplicateRegistrations(t *testing.T) {
	t.Parallel()
	redactor := redaction.NewRedactor()
	redactor.Register("dup-secret")
	redactor.Register("dup-secret")
	redactor.Register("   ")
	redactor.Register("")

	if got := redactor.Redact("dup-secret"); got != redaction.Placeholder {
		t.Fatalf("duplicate registration broke redaction: %s", got)
	}
}

func TestRedactorIsConcurrencySafe(t *testing.T) {
	t.Parallel()
	redactor := redaction.NewRedactor()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(2)
		secret := "parallel-secret-" + string(rune('a'+i%26))
		go func() { defer wg.Done(); redactor.Register(secret) }()
		go func() {
			defer wg.Done()
			// Every Redact call must observe a consistent phrase slice.
			_ = redactor.Redact("text with " + secret)
		}()
	}
	wg.Wait()
	if strings.Contains(redactor.Redact("parallel-secret-a parallel-secret-z"), "parallel-secret") {
		t.Fatal("concurrent registration lost a secret")
	}
}
