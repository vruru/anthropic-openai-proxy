package main

import "encoding/json"

// ── Anthropic Types ──────────────────────────────────────────────

type AnthropicRequest struct {
	Model         string          `json:"model"`
	System        json.RawMessage `json:"system,omitempty"` // string or array of {type, text}
	Messages      []AnthropicMessage `json:"messages"`
	Tools         []AnthropicTool `json:"tools,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	MaxTokens     int             `json:"max_tokens,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
}

type AnthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string or array of ContentBlock
}

type ContentBlock struct {
	Type       string          `json:"type"` // text, image, tool_use, tool_result
	Text       string          `json:"text,omitempty"`
	Source     *ImageSource    `json:"source,omitempty"`
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	ToolUseID  string          `json:"tool_use_id,omitempty"`
	Content    json.RawMessage `json:"content,omitempty"` // string or array for tool_result
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type AnthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type AnthropicResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence"`
	Usage        AnthropicUsage `json:"usage"`
}

type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ── OpenAI Types ─────────────────────────────────────────────────

type OAIRequest struct {
	Model       string       `json:"model"`
	Messages    []OAIMessage `json:"messages"`
	Tools       []OAITool    `json:"tools,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	TopP        *float64     `json:"top_p,omitempty"`
	Stop        []string     `json:"stop,omitempty"`
}

type OAIMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"` // string or multipart array
	ToolCalls  []OAIToolCall   `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

type OAIToolCall struct {
	Index    int         `json:"index"`
	ID       string      `json:"id,omitempty"`
	Type     string      `json:"type,omitempty"`
	Function OAIFunction `json:"function"`
}

type OAIFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OAITool struct {
	Type     string         `json:"type"`
	Function OAIToolFunction `json:"function"`
}

type OAIToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type OAIResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Choices []OAIChoice `json:"choices"`
	Usage   *OAIUsage   `json:"usage,omitempty"`
}

type OAIChoice struct {
	Index        int        `json:"index"`
	Message      OAIMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type OAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type OAIStreamChunk struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Choices []OAIStreamChoice `json:"choices"`
	Usage   *OAIUsage         `json:"usage,omitempty"`
}

type OAIStreamChoice struct {
	Index        int      `json:"index"`
	Delta        OAIDelta `json:"delta"`
	FinishReason string   `json:"finish_reason,omitempty"`
}

type OAIDelta struct {
	Role      string        `json:"role,omitempty"`
	Content   string        `json:"content,omitempty"`
	ToolCalls []OAIToolCall `json:"tool_calls,omitempty"`
}

// ── Anthropic SSE Event Types ────────────────────────────────────

type MessageStartEvent struct {
	Type    string                   `json:"type"`
	Message MessageStartEventMessage `json:"message"`
}

type MessageStartEventMessage struct {
	ID           string              `json:"id"`
	Type         string              `json:"type"`
	Role         string              `json:"role"`
	Content      []ContentBlock      `json:"content"`
	Model        string              `json:"model"`
	StopReason   *string             `json:"stop_reason"`
	StopSequence *string             `json:"stop_sequence"`
	Usage        MessageStartUsage   `json:"usage"`
}

type MessageStartUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type ContentBlockStartEvent struct {
	Type         string       `json:"type"`
	Index        int          `json:"index"`
	ContentBlock ContentBlock `json:"content_block"`
}

type ContentBlockDeltaEvent struct {
	Type  string      `json:"type"`
	Index int         `json:"index"`
	Delta DeltaBlock  `json:"delta"`
}

type DeltaBlock struct {
	Type        string `json:"type"` // text_delta or input_json_delta
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type ContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type MessageDeltaEvent struct {
	Type  string           `json:"type"`
	Delta MessageDelta     `json:"delta"`
	Usage MessageDeltaUsage `json:"usage"`
}

type MessageDelta struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

type MessageDeltaUsage struct {
	OutputTokens int `json:"output_tokens"`
}

type MessageStopEvent struct {
	Type string `json:"type"`
}

// ── Error Types ──────────────────────────────────────────────────

type ErrorResponse struct {
	Type  string       `json:"type"`
	Error ErrorDetails `json:"error"`
}

type ErrorDetails struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
