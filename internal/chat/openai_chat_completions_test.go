package chat_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jayj1997/gum-workflows/internal/chat"
)

// recordedRequest captures one fixture-server observation.
type recordedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   map[string]any
}

// fixtureServer returns an OpenAI-compatible fixture that records requests and
// replies with the canned response body.
func fixtureServer(t *testing.T, status int, responseBody string) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests = append(requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Auth: r.Header.Get("Authorization"), Body: body})
		w.WriteHeader(status)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func validResponseBody() string {
	return `{"id":"chatcmpl-fixture-1","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"Real model response."}}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19,"prompt_tokens_details":{"cached_tokens":3},"completion_tokens_details":{"reasoning_tokens":2,"cached_reasoning_tokens":1}}}`
}

func singleTurnRequest() chat.GenerateRequest {
	return chat.GenerateRequest{
		Model:        "gpt-fixture",
		Instructions: []chat.ContentPart{chat.TextPart("Answer tersely.")},
		Messages:     []chat.ChatMessage{chat.UserTextMessage("Hello from the product UI.")},
	}
}

func TestOpenAIChatAdapterMapsCanonicalSingleTurnRequest(t *testing.T) {
	server, requests := fixtureServer(t, http.StatusOK, validResponseBody())
	adapter := chat.NewOpenAIChatAdapter(server.Client())
	temperature, maxTokens := 0.8, 64
	request := singleTurnRequest()
	request.Config = chat.GenerationConfig{Temperature: &temperature, MaxOutputTokens: &maxTokens}

	result, err := adapter.Generate(context.Background(), chat.Connection{
		Protocol: chat.ProtocolOpenAIChatCompletions, BaseURL: server.URL + "/v1/",
		ProviderModelID: "gpt-fixture", APIKey: "sk-fixture-secret",
	}, request)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("requests = %d, want exactly one call", len(*requests))
	}
	got := (*requests)[0]
	if got.Method != http.MethodPost || got.Path != "/v1/chat/completions" {
		t.Fatalf("request = %s %s, want POST /v1/chat/completions", got.Method, got.Path)
	}
	if got.Auth != "Bearer sk-fixture-secret" {
		t.Fatalf("authorization = %q, want bearer fixture key", got.Auth)
	}
	// Golden request body: developer dialect instructions first, then the
	// user message in order, with generation parameters mapped.
	wantMessages := []any{
		map[string]any{"role": "developer", "content": "Answer tersely."},
		map[string]any{"role": "user", "content": "Hello from the product UI."},
	}
	if fmt.Sprint(got.Body["messages"]) != fmt.Sprint(wantMessages) {
		t.Fatalf("messages = %#v, want %#v", got.Body["messages"], wantMessages)
	}
	if got.Body["model"] != "gpt-fixture" || got.Body["temperature"] != 0.8 || got.Body["max_tokens"] != float64(64) {
		t.Fatalf("model/params = %#v", got.Body)
	}

	if result.Assistant.Role != "assistant" || result.Assistant.Text() != "Real model response." {
		t.Fatalf("assistant = %#v", result.Assistant)
	}
	if result.FinishReason != "stop" {
		t.Fatalf("finish reason = %q", result.FinishReason)
	}
	if result.ProviderRequestID != "chatcmpl-fixture-1" {
		t.Fatalf("provider request id = %q", result.ProviderRequestID)
	}
	if result.Usage.InputTokens != 12 || result.Usage.OutputTokens != 7 || result.Usage.TotalTokens != 19 {
		t.Fatalf("usage = %#v", result.Usage)
	}
	if result.Usage.CachedInputTokens != 3 || result.Usage.ReasoningTokens != 2 || result.Usage.CachedOutputTokens != 1 {
		t.Fatalf("usage details = %#v", result.Usage)
	}
}

func TestOpenAIChatAdapterSystemDialectAndBaseURLBoundaries(t *testing.T) {
	server, requests := fixtureServer(t, http.StatusOK, validResponseBody())
	cases := []struct {
		name     string
		baseURL  string
		role     chat.InstructionsRole
		wantRole string
		wantPath string
	}{
		{name: "trailing slash", baseURL: server.URL + "/v1/", wantRole: "developer", wantPath: "/v1/chat/completions"},
		{name: "no trailing slash", baseURL: server.URL + "/v1", wantRole: "developer", wantPath: "/v1/chat/completions"},
		{name: "reverse proxy subpath", baseURL: server.URL + "/llm/v1/", wantRole: "developer", wantPath: "/llm/v1/chat/completions"},
		{name: "root only", baseURL: server.URL + "/", wantRole: "developer", wantPath: "/chat/completions"},
		{name: "system dialect", baseURL: server.URL + "/v1/", role: chat.RoleSystem, wantRole: "system", wantPath: "/v1/chat/completions"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			adapter := chat.NewOpenAIChatAdapter(server.Client())
			adapter.InstructionsRole = testCase.role
			if _, err := adapter.Generate(context.Background(), chat.Connection{BaseURL: testCase.baseURL}, singleTurnRequest()); err != nil {
				t.Fatalf("generate: %v", err)
			}
			got := (*requests)[len(*requests)-1]
			if got.Path != testCase.wantPath {
				t.Fatalf("path = %q, want %q", got.Path, testCase.wantPath)
			}
			messages := got.Body["messages"].([]any)
			if messages[0].(map[string]any)["role"] != testCase.wantRole {
				t.Fatalf("instructions role = %#v, want %q", messages[0], testCase.wantRole)
			}
		})
	}
}

func TestOpenAIChatAdapterClassifiesStructuralErrors(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		wantKind chat.OpenAIErrorKind
	}{
		{name: "authentication", status: http.StatusUnauthorized, body: `{"error":{"message":"Incorrect API key"}}`, wantKind: chat.ErrAuth},
		{name: "forbidden", status: http.StatusForbidden, body: `{}`, wantKind: chat.ErrAuth},
		{name: "rate limit", status: http.StatusTooManyRequests, body: `{"error":{"message":"Rate limit reached"}}`, wantKind: chat.ErrRateLimit},
		{name: "provider unavailable", status: http.StatusServiceUnavailable, body: `{}`, wantKind: chat.ErrProvider},
		{name: "provider rejects request", status: http.StatusBadRequest, body: `{"error":{"message":"model does not support image"}}`, wantKind: chat.ErrProvider},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server, _ := fixtureServer(t, testCase.status, testCase.body)
			adapter := chat.NewOpenAIChatAdapter(server.Client())
			_, err := adapter.Generate(context.Background(), chat.Connection{BaseURL: server.URL + "/v1"}, singleTurnRequest())
			var openAIError *chat.OpenAIError
			if !errors.As(err, &openAIError) {
				t.Fatalf("error = %v, want OpenAIError", err)
			}
			if openAIError.Kind != testCase.wantKind || openAIError.StatusCode != testCase.status {
				t.Fatalf("error = %v, want kind %q status %d", err, testCase.wantKind, testCase.status)
			}
			if openAIError.ProviderMessage != "" && !strings.Contains(testCase.body, openAIError.ProviderMessage) {
				t.Fatalf("provider message = %q not from fixture", openAIError.ProviderMessage)
			}
		})
	}
}

func TestOpenAIChatAdapterRejectsMalformedResponses(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "not json", body: `<html>maintenance</html>`},
		{name: "missing id", body: `{"choices":[{"message":{"role":"assistant","content":"hi"}}]}`},
		{name: "no choices", body: `{"id":"x","choices":[]}`},
		{name: "empty content", body: `{"id":"x","choices":[{"message":{"role":"assistant","content":""}}]}`},
		{name: "non-text parts only", body: `{"id":"x","choices":[{"message":{"role":"assistant","content":[{"type":"image_url"}]}}]}`},
		{name: "wrong role", body: `{"id":"x","choices":[{"message":{"role":"tool","content":"hi"}}]}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server, _ := fixtureServer(t, http.StatusOK, testCase.body)
			adapter := chat.NewOpenAIChatAdapter(server.Client())
			_, err := adapter.Generate(context.Background(), chat.Connection{BaseURL: server.URL}, singleTurnRequest())
			var openAIError *chat.OpenAIError
			if !errors.As(err, &openAIError) || openAIError.Kind != chat.ErrMalformedResponse {
				t.Fatalf("error = %v, want malformed-response", err)
			}
			if strings.Contains(err.Error(), "sk-") {
				t.Fatalf("error leaks secrets: %v", err)
			}
		})
	}
}

func TestOpenAIChatAdapterSurfacesNetworkFailureWithoutSecrets(t *testing.T) {
	// A closed server port yields a transport error without real network access.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()
	adapter := chat.NewOpenAIChatAdapter(nil)
	_, err := adapter.Generate(context.Background(), chat.Connection{BaseURL: url, APIKey: "sk-network-secret"}, singleTurnRequest())
	var openAIError *chat.OpenAIError
	if !errors.As(err, &openAIError) || openAIError.Kind != chat.ErrNetwork {
		t.Fatalf("error = %v, want network OpenAIError", err)
	}
	if strings.Contains(err.Error(), "sk-network-secret") {
		t.Fatalf("network error leaks API key: %v", err)
	}
}

func TestOpenAIChatAdapterReturnsContextCancellation(t *testing.T) {
	server, _ := fixtureServer(t, http.StatusOK, validResponseBody())
	adapter := chat.NewOpenAIChatAdapter(server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := adapter.Generate(ctx, chat.Connection{BaseURL: server.URL}, singleTurnRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestOpenAIChatAdapterWaitsForFullResponseBody(t *testing.T) {
	// The fixture writes the body in two chunks; the adapter must read the
	// complete body before producing a result.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-2","choices":[{"index":0,"finish_reason":"stop","message":{"role":"`))
		<-release
		_, _ = w.Write([]byte(`assistant","content":"delayed but complete."}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))
	t.Cleanup(server.Close)
	adapter := chat.NewOpenAIChatAdapter(server.Client())
	done := make(chan chat.GenerateResult, 1)
	go func() {
		result, err := adapter.Generate(context.Background(), chat.Connection{BaseURL: server.URL}, singleTurnRequest())
		if err != nil {
			t.Errorf("generate: %v", err)
		}
		done <- result
	}()
	select {
	case <-done:
		t.Fatal("result produced before the provider response completed")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case result := <-done:
		if result.Assistant.Text() != "delayed but complete." {
			t.Fatalf("assistant = %#v", result.Assistant)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("generate did not complete after the response finished")
	}
}
