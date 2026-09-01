package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Protocol identifiers of the first product closure.
const (
	// ProtocolOpenAIChatCompletions is the OpenAI-compatible Chat Completions protocol.
	ProtocolOpenAIChatCompletions = "openai-chat-completions"
)

// OpenAIErrorKind classifies protocol-level failures as Structural Errors.
type OpenAIErrorKind string

// Error kinds surfaced by the OpenAI-compatible adapter.
const (
	ErrAuth              OpenAIErrorKind = "authentication"
	ErrRateLimit         OpenAIErrorKind = "rate-limit"
	ErrProvider          OpenAIErrorKind = "provider"
	ErrNetwork           OpenAIErrorKind = "network"
	ErrMalformedResponse OpenAIErrorKind = "malformed-response"
)

// OpenAIError is a Structural Error from a real OpenAI-compatible call. It
// carries no request headers, secrets or response bodies.
type OpenAIError struct {
	Kind       OpenAIErrorKind
	StatusCode int
	// ProviderMessage is a sanitized provider error message (error.message of
	// the wire error object); it never includes the Authorization header.
	ProviderMessage string
	Err             error
}

func (e *OpenAIError) Error() string {
	var builder strings.Builder
	builder.WriteString("openai-compatible request failed: ")
	builder.WriteString(string(e.Kind))
	if e.StatusCode != 0 {
		fmt.Fprintf(&builder, " (status %d)", e.StatusCode)
	}
	if e.ProviderMessage != "" {
		builder.WriteString(": ")
		builder.WriteString(e.ProviderMessage)
	}
	if e.Err != nil {
		builder.WriteString(": ")
		builder.WriteString(e.Err.Error())
	}
	return builder.String()
}

func (e *OpenAIError) Unwrap() error { return e.Err }

// OpenAIChatAdapter talks the OpenAI-compatible non-streaming Chat
// Completions protocol. It works through an injectable *http.Client so tests
// run against a local fixture server without real network access.
type OpenAIChatAdapter struct {
	client *http.Client
	// InstructionsRole maps canonical Instructions to developer (default) or
	// system messages per Provider dialect.
	InstructionsRole InstructionsRole
}

// NewOpenAIChatAdapter returns an adapter using the given HTTP client. A nil
// client falls back to http.DefaultClient with a sane timeout.
func NewOpenAIChatAdapter(client *http.Client) *OpenAIChatAdapter {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &OpenAIChatAdapter{client: client, InstructionsRole: RoleDeveloper}
}

// chatCompletionRequest is the OpenAI Chat Completions wire request. It is
// private to this file so canonical types never leak provider fields.
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []wireMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
}

type wireMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type wireContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type chatCompletionResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		TotalTokens         int `json:"total_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails struct {
			ReasoningTokens       int `json:"reasoning_tokens"`
			CachedReasoningTokens int `json:"cached_reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

// Generate performs one non-streaming Chat Completions call.
func (a *OpenAIChatAdapter) Generate(ctx context.Context, conn Connection, req GenerateRequest) (GenerateResult, error) {
	endpoint, err := chatCompletionsURL(conn.BaseURL)
	if err != nil {
		return GenerateResult{}, err
	}
	wire, err := buildChatCompletionRequest(a.InstructionsRole, req)
	if err != nil {
		return GenerateResult{}, err
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return GenerateResult{}, fmt.Errorf("openai-compatible request: encode body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return GenerateResult{}, &OpenAIError{Kind: ErrNetwork, Err: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if conn.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+conn.APIKey)
	}
	httpResp, err := a.client.Do(httpReq)
	if err != nil {
		kind := ErrNetwork
		if ctx.Err() != nil {
			return GenerateResult{}, ctx.Err()
		}
		return GenerateResult{}, &OpenAIError{Kind: kind, Err: err}
	}
	defer func() { _ = httpResp.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(httpResp.Body, 32<<20))
	if err != nil {
		return GenerateResult{}, &OpenAIError{Kind: ErrNetwork, StatusCode: httpResp.StatusCode, Err: err}
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return GenerateResult{}, newOpenAIStatusError(httpResp.StatusCode, responseBody)
	}
	var decoded chatCompletionResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return GenerateResult{}, &OpenAIError{Kind: ErrMalformedResponse, StatusCode: httpResp.StatusCode, Err: fmt.Errorf("decode response body: %w", err)}
	}
	return openAIResult(decoded)
}

func newOpenAIStatusError(status int, body []byte) *OpenAIError {
	kind := ErrProvider
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		kind = ErrAuth
	case status == http.StatusTooManyRequests:
		kind = ErrRateLimit
	case status >= 500:
		kind = ErrProvider
	}
	message := ""
	var wireError struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &wireError); err == nil && wireError.Error.Message != "" {
		message = wireError.Error.Message
	}
	return &OpenAIError{Kind: kind, StatusCode: status, ProviderMessage: message}
}

func openAIResult(decoded chatCompletionResponse) (GenerateResult, error) {
	if decoded.ID == "" {
		return GenerateResult{}, &OpenAIError{Kind: ErrMalformedResponse, Err: fmt.Errorf("response id is missing")}
	}
	if len(decoded.Choices) == 0 {
		return GenerateResult{}, &OpenAIError{Kind: ErrMalformedResponse, Err: fmt.Errorf("response has no choices")}
	}
	choice := decoded.Choices[0]
	parts, err := decodeAssistantContent(choice.Message.Content)
	if err != nil {
		return GenerateResult{}, &OpenAIError{Kind: ErrMalformedResponse, Err: err}
	}
	if len(parts) == 0 {
		return GenerateResult{}, &OpenAIError{Kind: ErrMalformedResponse, Err: fmt.Errorf("assistant message has no text content")}
	}
	if choice.Message.Role != "" && choice.Message.Role != "assistant" {
		return GenerateResult{}, &OpenAIError{Kind: ErrMalformedResponse, Err: fmt.Errorf("choice role %q is not assistant", choice.Message.Role)}
	}
	usage := Usage{
		InputTokens:        decoded.Usage.PromptTokens,
		OutputTokens:       decoded.Usage.CompletionTokens,
		TotalTokens:        decoded.Usage.TotalTokens,
		ReasoningTokens:    decoded.Usage.CompletionTokensDetails.ReasoningTokens,
		CachedInputTokens:  decoded.Usage.PromptTokensDetails.CachedTokens,
		CachedOutputTokens: decoded.Usage.CompletionTokensDetails.CachedReasoningTokens,
	}
	return GenerateResult{
		Assistant:         ChatMessage{Role: "assistant", Parts: parts},
		FinishReason:      choice.FinishReason,
		Usage:             usage,
		ProviderRequestID: decoded.ID,
	}, nil
}

func decodeAssistantContent(raw json.RawMessage) ([]ContentPart, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("assistant message content is missing")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text == "" {
			return nil, fmt.Errorf("assistant message content is empty")
		}
		return []ContentPart{TextPart(text)}, nil
	}
	var parts []wireContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("assistant message content is neither string nor parts: %w", err)
	}
	result := make([]ContentPart, 0, len(parts))
	for _, part := range parts {
		if part.Type != "text" {
			continue
		}
		result = append(result, TextPart(part.Text))
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("assistant message content has no text parts")
	}
	return result, nil
}

func buildChatCompletionRequest(role InstructionsRole, req GenerateRequest) (chatCompletionRequest, error) {
	if strings.TrimSpace(req.Model) == "" {
		return chatCompletionRequest{}, fmt.Errorf("openai-compatible request: model must not be empty")
	}
	instructionRole := string(role)
	if instructionRole == "" {
		instructionRole = string(RoleDeveloper)
	}
	var messages []wireMessage
	for _, part := range req.Instructions {
		if strings.TrimSpace(part.Text) == "" {
			continue
		}
		encoded, err := json.Marshal(part.Text)
		if err != nil {
			return chatCompletionRequest{}, fmt.Errorf("openai-compatible request: encode instructions: %w", err)
		}
		messages = append(messages, wireMessage{Role: instructionRole, Content: encoded})
	}
	for _, message := range req.Messages {
		encoded, err := encodeParts(message.Parts)
		if err != nil {
			return chatCompletionRequest{}, fmt.Errorf("openai-compatible request: encode %s message: %w", message.Role, err)
		}
		messages = append(messages, wireMessage{Role: message.Role, Content: encoded})
	}
	return chatCompletionRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Config.Temperature,
		MaxTokens:   req.Config.MaxOutputTokens,
	}, nil
}

func encodeParts(parts []ContentPart) (json.RawMessage, error) {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Kind != PartText {
			return nil, fmt.Errorf("content part kind %q is not supported", part.Kind)
		}
		texts = append(texts, part.Text)
	}
	if len(texts) == 1 {
		return json.Marshal(texts[0])
	}
	return json.Marshal(strings.Join(texts, "\n"))
}

// chatCompletionsURL joins the Chat Completions path onto the protocol API
// root using the URL parser; string concatenation is forbidden because it
// cannot honor trailing-slash and sub-path base URLs.
func chatCompletionsURL(baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", &OpenAIError{Kind: ErrNetwork, Err: fmt.Errorf("parse base URL: %w", err)}
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", &OpenAIError{Kind: ErrNetwork, Err: fmt.Errorf("base URL must be absolute")}
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + "/chat/completions"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
