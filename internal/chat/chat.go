// Package chat defines the canonical product conversation model and the
// Protocol Adapter seam used by real model calls. It is provider-independent:
// no OpenAI or Anthropic wire field may appear on these types.
package chat

import (
	"context"
)

// ContentPart is one typed part of a message body. The first product closure
// only carries text; multimodal parts enter later through new kinds.
type ContentPart struct {
	Kind string `json:"kind"`
	Text string `json:"text,omitempty"`
}

// ContentPart kinds of the first product closure.
const (
	// PartText is the plain text content part.
	PartText = "text"
)

// TextPart returns one text ContentPart.
func TextPart(text string) ContentPart {
	return ContentPart{Kind: PartText, Text: text}
}

// ChatMessage is one user- or assistant-visible conversation message. Role is
// "user" or "assistant"; system guidance travels separately as Instructions so
// business conversations never contain hidden instruction messages.
type ChatMessage struct {
	Role  string        `json:"role"`
	Parts []ContentPart `json:"parts"`
}

// Text returns the concatenated text content of the message.
func (m ChatMessage) Text() string {
	text := ""
	for _, part := range m.Parts {
		if part.Kind == PartText {
			if text != "" {
				text += "\n"
			}
			text += part.Text
		}
	}
	return text
}

// UserTextMessage returns one user message with a single text part.
func UserTextMessage(text string) ChatMessage {
	return ChatMessage{Role: "user", Parts: []ContentPart{TextPart(text)}}
}

// AssistantTextMessage returns one assistant message with a single text part.
func AssistantTextMessage(text string) ChatMessage {
	return ChatMessage{Role: "assistant", Parts: []ContentPart{TextPart(text)}}
}

// Conversation is the canonical Artifact body persisted by chat Nodes.
type Conversation struct {
	Messages []ChatMessage `json:"messages"`
}

// GenerationConfig carries effective generation parameters for one request.
// Zero fields mean "omit" so provider defaults apply.
type GenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

// GenerateRequest is the canonical, provider-independent model call request.
type GenerateRequest struct {
	Model        string
	Instructions []ContentPart
	Messages     []ChatMessage
	Config       GenerationConfig
}

// Usage reports the token accounting returned by one completed call.
type Usage struct {
	InputTokens        int `json:"inputTokens"`
	OutputTokens       int `json:"outputTokens"`
	TotalTokens        int `json:"totalTokens"`
	ReasoningTokens    int `json:"reasoningTokens,omitempty"`
	CachedInputTokens  int `json:"cachedInputTokens,omitempty"`
	CachedOutputTokens int `json:"cachedOutputTokens,omitempty"`
}

// GenerateResult is the canonical, provider-independent call outcome.
type GenerateResult struct {
	Assistant    ChatMessage
	FinishReason string
	Usage        Usage
	// ProviderRequestID is the provider's own request identifier for
	// diagnostics; it is never a Gum identity.
	ProviderRequestID string
}

// Connection carries the non-secret and secret connection facts resolved by
// the Application for one Run. APIKey is the resolved secret value; it must
// never be persisted, logged or returned in views.
type Connection struct {
	Protocol         string
	InstructionsRole InstructionsRole
	BaseURL          string
	ProviderModelID  string
	APIKey           string
}

// InstructionsRole selects how a protocol adapter maps Instructions.
type InstructionsRole string

// Instruction roles supported by the OpenAI-compatible adapter.
const (
	// RoleDeveloper maps instructions to a developer message.
	RoleDeveloper InstructionsRole = "developer"
	// RoleSystem maps instructions to a system message.
	RoleSystem InstructionsRole = "system"
)

// Adapter is the protocol seam: canonical request in, canonical result out.
// P10 implements non-streaming only; a streaming seam is added separately.
// Implementations must return structured errors whose messages and unwrap
// chains contain no resolved credential or sensitive request header.
type Adapter interface {
	Generate(ctx context.Context, conn Connection, req GenerateRequest) (GenerateResult, error)
}
