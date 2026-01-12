package openai

import (
	"encoding/json"
)

type ApiChatCompletionsRequest struct {
	Model             string     `json:"model"`
	Messages          []*Message `json:"messages" binding:"required"`
	Thinking          *Thinking  `json:"thinking,omitempty"`
	EnableThinking    *bool      `json:"enable_thinking,omitempty"`
	ReasoningSplit    *bool      `json:"reasoning_split,omitempty"`
	MaxTokens         *int       `json:"max_tokens,omitempty"`
	Temperature       *float64   `json:"temperature,omitempty"`
	TopP              *float64   `json:"top_p,omitempty"`
	TopK              *int       `json:"top_k,omitempty"`
	FrequencyPenalty  *float64   `json:"frequency_penalty,omitempty"`
	PresencePenalty   *float64   `json:"presence_penalty,omitempty"`
	RepetitionPenalty *float64   `json:"repetition_penalty,omitempty"`
	Stop              []string   `json:"stop,omitempty"`
	Stream            bool       `json:"stream,omitempty"`
	LogProbs          *bool      `json:"logprobs,omitempty"`
	TopLogProbs       *int       `json:"top_logprobs,omitempty"`
	N                 *int       `json:"n,omitempty"`
	Tools             []Tool     `json:"tools,omitempty"` // 可用函数工具列表

	ToolChoice          *ToolChoice        `json:"tool_choice,omitempty"`         // 指定强制调用的函数（可选）
	ParallelToolCalls   *bool              `json:"parallel_tool_calls,omitempty"` // 是否并行调用工具
	ResponseFormat      *ResponseFormat    `json:"response_format,omitempty"`
	ReasoningEffort     *string            `json:"reasoning_effort,omitempty"`
	User                *string            `json:"user,omitempty"`           // 用户标识符
	Seed                *int               `json:"seed,omitempty"`           // 可复现随机种子
	StreamOptions       *StreamOptions     `json:"stream_options,omitempty"` // 流式配置
	ServiceTier         *string            `json:"service_tier,omitempty"`   // 服务层级
	Store               *bool              `json:"store,omitempty"`          // 是否存储对话
	Metadata            *map[string]string `json:"metadata,omitempty"`       // 元数据
	MaxCompletionTokens *int64             `json:"max_completion_tokens,omitempty"`

	ExtraBody *ExtraBody `json:"extra_body,omitempty"`
	ExtraPart *ExtraPart `json:"extra_part,omitempty"`
}

type Thinking struct {
	Type string `json:"type"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"` // 是否在流式响应中包含 usage 信息
}

type ResponseFormat struct {
	Type       string           `json:"type" binding:"required"`
	JsonSchema *json.RawMessage `json:"json_schema,omitempty"`
}

type Message struct {
	Role             string         `json:"role" binding:"required"`
	Content          MessageContent `json:"content,omitempty"`
	ReasoningContent MessageContent `json:"reasoning_content,omitempty"`
	Name             string         `json:"name,omitempty"`
	ToolCalls        []*ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
}

// UnmarshalJSON 实现 Message 的自定义 JSON 反序列化
func (m *Message) UnmarshalJSON(data []byte) error {
	// 定义一个临时结构体，Content 和 ReasoningContent 使用 RawMessage
	type Alias struct {
		Role             string          `json:"role"`
		Content          json.RawMessage `json:"content,omitempty"`
		ReasoningContent json.RawMessage `json:"reasoning_content,omitempty"`
		Name             string          `json:"name,omitempty"`
		ToolCalls        []*ToolCall     `json:"tool_calls,omitempty"`
		ToolCallID       string          `json:"tool_call_id,omitempty"`
	}

	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	m.Role = alias.Role
	m.Name = alias.Name
	m.ToolCalls = alias.ToolCalls
	m.ToolCallID = alias.ToolCallID

	// 解析 Content 字段
	if len(alias.Content) > 0 {
		content, err := unmarshalMessageContent(alias.Content)
		if err != nil {
			return err
		}
		m.Content = content
	}

	// 解析 ReasoningContent 字段
	if len(alias.ReasoningContent) > 0 {
		reasoningContent, err := unmarshalMessageContent(alias.ReasoningContent)
		if err != nil {
			return err
		}
		m.ReasoningContent = reasoningContent
	}

	return nil
}

// unmarshalMessageContent 根据 JSON 数据类型解析 MessageContent
func unmarshalMessageContent(data json.RawMessage) (MessageContent, error) {
	// 尝试解析为字符串
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		return MessageContentString(str), nil
	}

	// 尝试解析为数组
	var arr []*MessageContentItem
	if err := json.Unmarshal(data, &arr); err == nil {
		return MessageContentArray(arr), nil
	}

	return nil, nil
}

// MarshalJSON 实现 Message 的自定义 JSON 序列化
func (m Message) MarshalJSON() ([]byte, error) {
	type Alias struct {
		Role             string          `json:"role"`
		Content          json.RawMessage `json:"content,omitempty"`
		ReasoningContent json.RawMessage `json:"reasoning_content,omitempty"`
		Name             string          `json:"name,omitempty"`
		ToolCalls        []*ToolCall     `json:"tool_calls,omitempty"`
		ToolCallID       string          `json:"tool_call_id,omitempty"`
	}

	alias := Alias{
		Role:       m.Role,
		Name:       m.Name,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
	}

	// 序列化 Content
	if m.Content != nil {
		contentBytes, err := json.Marshal(m.Content)
		if err != nil {
			return nil, err
		}
		alias.Content = contentBytes
	}

	// 序列化 ReasoningContent
	if m.ReasoningContent != nil {
		reasoningBytes, err := json.Marshal(m.ReasoningContent)
		if err != nil {
			return nil, err
		}
		alias.ReasoningContent = reasoningBytes
	}

	return json.Marshal(alias)
}

type ExtraBody struct {
	Google json.RawMessage `json:"google,omitempty"`
}

type ExtraPart struct {
	Google json.RawMessage `json:"google,omitempty"`
}

type MessageContent interface {
	ExtractText() string
}

type MessageContentString string

func (m MessageContentString) ExtractText() string { return string(m) }
func (m MessageContentString) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(m))
}

type MessageContentArray []*MessageContentItem

func (m MessageContentArray) ExtractText() string {
	text := ""
	for _, item := range m {
		if item.Type == "text" {
			text += item.Text
		}
	}
	return text
}
func (m MessageContentArray) MarshalJSON() ([]byte, error) {
	return json.Marshal([]*MessageContentItem(m))
}

type MessageContentItem struct {
	Type       string                        `json:"type" binding:"required"`
	Text       string                        `json:"text,omitempty"`
	ImageURL   *MessageContentItemImageURL   `json:"image_url,omitempty"`
	VideoURL   *MessageContentItemVideoURL   `json:"video_url,omitempty"`
	AudioURL   *MessageContentItemAudioURL   `json:"audio_url,omitempty"`
	InputAudio *MessageContentItemInputAudio `json:"input_audio,omitempty"`
	File       *MessageContentItemFile       `json:"file,omitempty"` // Added for Gemini file support
}

type MessageContentItemImageURL struct {
	URL    string `json:"url" binding:"required"`
	Detail string `json:"detail,omitempty"`
}

type MessageContentItemVideoURL struct {
	URL string `json:"url" binding:"required"`
}

type MessageContentItemAudioURL struct {
	URL string `json:"url" binding:"required"`
}

type MessageContentItemInputAudio struct {
	Data   string `json:"data" binding:"required"`
	Format string `json:"format" binding:"required"`
}

// Added for Gemini file support
type MessageContentItemFile struct {
	FileURI  string `json:"file_uri" binding:"required"`
	MIMEType string `json:"mime_type,omitempty"`
}

// Tool 可用工具定义
type Tool struct {
	Type     string       `json:"type,omitempty"`     // 类型（固定为"function"）
	Function ToolFunction `json:"function,omitempty"` // 函数元数据
}

// ToolChoice 工具选择策略
// 支持字符串形式："auto", "none", "required"
// 或对象形式：{"type": "function", "function": {"name": "xxx"}}
type ToolChoice struct {
	// 字符串形式
	String *string `json:"-"`
	// 对象形式
	Object *ToolChoiceObject `json:"-"`
}

type ToolChoiceObject struct {
	Type     string              `json:"type"`     // 固定为 "function"
	Function *ToolChoiceFunction `json:"function"` // 函数选择
}

type ToolChoiceFunction struct {
	Name string `json:"name"` // 函数名称
}

// MarshalJSON 实现 ToolChoice 的 JSON 序列化
func (tc ToolChoice) MarshalJSON() ([]byte, error) {
	if tc.String != nil {
		return json.Marshal(*tc.String)
	}
	if tc.Object != nil {
		return json.Marshal(tc.Object)
	}
	return []byte("null"), nil
}

// UnmarshalJSON 实现 ToolChoice 的 JSON 反序列化
func (tc *ToolChoice) UnmarshalJSON(data []byte) error {
	// 尝试解析为字符串
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		tc.String = &s
		return nil
	}

	// 尝试解析为对象
	var obj ToolChoiceObject
	if err := json.Unmarshal(data, &obj); err == nil {
		tc.Object = &obj
		return nil
	}

	return nil
}

// ToolFunction 函数元数据（名称、描述、参数Schema）
type ToolFunction struct {
	Name        *string         `json:"name,omitempty"`        // 函数唯一标识
	Description *string         `json:"description,omitempty"` // 功能描述
	Parameters  json.RawMessage `json:"parameters,omitempty"`  // JSON Schema格式参数定义
}

type ToolCall struct {
	ID       string            `json:"id" binding:"required"`
	Index    int               `json:"index" binding:"required"`
	Type     string            `json:"type" binding:"required"`
	Function *ToolCallFunction `json:"function" binding:"required"`
}

type ToolCallFunction struct {
	Name      string `json:"name" binding:"required"`
	Arguments string `json:"arguments" binding:"required"`
}
