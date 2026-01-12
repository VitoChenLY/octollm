package anthropic

import (
	"encoding/json"
)

// ApiMessagesResponse represents a complete Anthropic Messages API response
type ApiMessagesResponse struct {
	ID           string           `json:"id"`
	Type         string           `json:"type"`
	Role         string           `json:"role"`
	Content      []MessageContent `json:"content"`
	Model        string           `json:"model"`
	StopReason   string           `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence,omitempty"`
	Usage        *Usage           `json:"usage"`
}

// Usage represents token usage information
type Usage struct {
	// Total input tokens
	InputTokens int64 `json:"input_tokens"`

	// Total output tokens
	OutputTokens int64 `json:"output_tokens"`

	// Tokens from cache creation (prompt caching)
	CacheCreationInputTokens *int64 `json:"cache_creation_input_tokens,omitempty"`

	// Tokens from cache read (prompt caching)
	CacheReadInputTokens *int64 `json:"cache_read_input_tokens,omitempty"`
}

// ApiMessagesStreamEvent represents a streaming event
// Aligned with MessageStreamEventUnion from SDK
type ApiMessagesStreamEvent struct {
	// Event type: "message_start", "content_block_start", "content_block_delta",
	// "content_block_stop", "message_delta", "message_stop", "ping", "error"
	Type string `json:"type"`

	// For message_start event
	Message *ApiMessagesResponse `json:"message,omitempty"`

	// For content_block_start event
	Index *int `json:"index,omitempty"`

	ContentBlock MessageContent  `json:"content_block,omitempty"`
	DeltaRaw     json.RawMessage `json:"delta,omitempty"`
	Usage        *Usage          `json:"usage,omitempty"`
	Error        *APIError       `json:"error,omitempty"`
}

// GetContentBlockDelta returns the delta as ApiContentBlockDelta if applicable
func (e *ApiMessagesStreamEvent) GetContentBlockDelta() (*ApiContentBlockDelta, error) {
	if e.Type != "content_block_delta" || len(e.DeltaRaw) == 0 {
		return nil, nil
	}
	var delta ApiContentBlockDelta
	if err := json.Unmarshal(e.DeltaRaw, &delta); err != nil {
		return nil, err
	}
	return &delta, nil
}

// GetMessageDelta returns the delta as ApiMessageDelta if applicable
func (e *ApiMessagesStreamEvent) GetMessageDelta() (*ApiMessageDelta, error) {
	if e.Type != "message_delta" || len(e.DeltaRaw) == 0 {
		return nil, nil
	}
	var delta ApiMessageDelta
	if err := json.Unmarshal(e.DeltaRaw, &delta); err != nil {
		return nil, err
	}
	return &delta, nil
}

// ApiContentBlockDelta represents incremental content updates
type ApiContentBlockDelta struct {
	Type string `json:"type"` // "text_delta", "input_json_delta", "thinking_delta"

	// For text_delta
	Text *string `json:"text,omitempty"`

	// For input_json_delta (tool use)
	PartialJSON *string `json:"partial_json,omitempty"`

	// For thinking_delta
	Thinking *string `json:"thinking,omitempty"`
}

// ApiMessageDelta represents message-level delta updates
type ApiMessageDelta struct {
	StopReason   *string `json:"stop_reason,omitempty"`
	StopSequence *string `json:"stop_sequence,omitempty"`
}

// APIError represents an error response
type APIError struct {
	Type    string `json:"type"` // e.g., "invalid_request_error"
	Message string `json:"message"`
}

// ExtractText extracts all text content from the response
func (r *ApiMessagesResponse) ExtractText() string {
	text := ""
	for _, block := range r.Content {
		if blockBlock, ok := block.(*MessageContentBlock); ok {
			if blockBlock.Type == "text" && blockBlock.Text != nil {
				text += *blockBlock.Text
			}
			if blockBlock.Type == "thinking" && blockBlock.MessageContentThinking != nil {
				text += blockBlock.MessageContentThinking.Thinking
			}
		} else if str, ok := block.(MessageContentString); ok {
			text += string(str)
		}
	}
	return text
}

// ExtractToolUses extracts all tool use blocks from the response
func (r *ApiMessagesResponse) ExtractToolUses() []*MessageContentToolUse {
	var toolUses []*MessageContentToolUse
	for _, block := range r.Content {
		if blockBlock, ok := block.(*MessageContentBlock); ok && blockBlock.Type == "tool_use" && blockBlock.MessageContentToolUse != nil {
			toolUses = append(toolUses, blockBlock.MessageContentToolUse)
		}
	}
	return toolUses
}

// IsToolUse checks if the response contains tool use
func (r *ApiMessagesResponse) IsToolUse() bool {
	return r.StopReason == "tool_use"
}

// IsError checks if this is an error event
func (e *ApiMessagesStreamEvent) IsError() bool {
	return e.Type == "error"
}

// IsMessageStart checks if this is a message start event
func (e *ApiMessagesStreamEvent) IsMessageStart() bool {
	return e.Type == "message_start"
}

// IsMessageStop checks if this is a message stop event
func (e *ApiMessagesStreamEvent) IsMessageStop() bool {
	return e.Type == "message_stop"
}

// IsContentBlockDelta checks if this is a content block delta event
func (e *ApiMessagesStreamEvent) IsContentBlockDelta() bool {
	return e.Type == "content_block_delta"
}
