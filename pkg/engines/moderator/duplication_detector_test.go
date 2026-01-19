package moderator

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/infinigence/octollm/pkg/octollm"
	"github.com/infinigence/octollm/pkg/types/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockEngine 用于测试的 mock engine
type mockEngine struct {
	response *octollm.Response
	err      error
}

func (m *mockEngine) Process(req *octollm.Request) (*octollm.Response, error) {
	return m.response, m.err
}

// 辅助函数：创建使用 TextModeratorEngine 的重复检测器
func newDuplicationDetectorEngine(config *DuplicationDetectorConfig, modelName string, next octollm.Engine) *TextModeratorEngine {
	service := NewDuplicationDetectorService(config, modelName)
	return &TextModeratorEngine{
		ModeratorService:     service,
		TextModeratorAdapter: &OpenAIAdapter{},
		ModerateInput:        false,
		ModerateOutput:       true,
		ModerateStreamEvery:  10,
		Next:                 next,
	}
}

func TestDuplicationDetector_NonStream_WithRepetition(t *testing.T) {
	// 创建一个包含重复内容的响应
	repeatedText := strings.Repeat("这是一个测试文本。", 5) // 重复 5 次
	resp := &openai.ChatCompletionResponse{
		ID:      "test-123",
		Model:   "gpt-4",
		Choices: []*openai.ChatCompletionChoice{
			{
				Message: &openai.Message{
					Content: openai.MessageContentString(repeatedText),
				},
			},
		},
	}

	mockResp := &octollm.Response{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionResponse]{}),
	}
	mockResp.Body.SetParsed(resp)

	mockEngine := &mockEngine{response: mockResp}

	detector := newDuplicationDetectorEngine(
		&DuplicationDetectorConfig{
			MinRepeatLen:    5,
			MaxRepeatLen:    50,
			RepeatThreshold: 3,
			DetectTimeout:   1 * time.Second,
		},
		"gpt-4",
		mockEngine,
	)

	httpReq, _ := http.NewRequestWithContext(context.Background(), "POST", "http://localhost/v1/chat/completions", nil)
	httpReq.URL, _ = url.Parse("http://localhost/v1/chat/completions")
	req := octollm.NewRequest(httpReq, octollm.APIFormatChatCompletions)
	req.Body = octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{})
	req.Body.SetParsed(&openai.ChatCompletionRequest{Model: "gpt-4"})

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Body)

	// 给异步检测一些时间
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_NonStream_NoRepetition(t *testing.T) {
	// 创建一个不包含重复的响应
	normalText := "这是一段正常的文本，没有任何重复的内容。"
	resp := &openai.ChatCompletionResponse{
		ID:      "test-456",
		Model:   "gpt-4",
		Choices: []*openai.ChatCompletionChoice{
			{
				Message: &openai.Message{
					Content: openai.MessageContentString(normalText),
				},
			},
		},
	}

	mockResp := &octollm.Response{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionResponse]{}),
	}
	mockResp.Body.SetParsed(resp)

	mockEngine := &mockEngine{response: mockResp}

	detector := newDuplicationDetectorEngine(
		&DuplicationDetectorConfig{
			MinRepeatLen:    5,
			MaxRepeatLen:    50,
			RepeatThreshold: 3,
			DetectTimeout:   1 * time.Second,
		},
		"gpt-4",
		mockEngine,
	)

	httpReq, _ := http.NewRequestWithContext(context.Background(), "POST", "http://localhost/v1/chat/completions", nil)
	httpReq.URL, _ = url.Parse("http://localhost/v1/chat/completions")
	req := octollm.NewRequest(httpReq, octollm.APIFormatChatCompletions)
	req.Body = octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{})
	req.Body.SetParsed(&openai.ChatCompletionRequest{Model: "gpt-4"})

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Body)

	// 给异步检测一些时间
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_Stream_WithRepetition(t *testing.T) {
	// 创建流式响应的 mock
	chunks := make(chan *octollm.StreamChunk, 10)

	// 模拟流式响应：发送重复的文本
	go func() {
		defer close(chunks)
		repeatedText := "重复"
		for i := 0; i < 60; i++ { // 发送 60 次
			chunk := &openai.ChatCompletionStreamChunk{
				ID:    "test-stream",
				Model: "gpt-4",
				Choices: []*openai.ChatCompletionStreamChoice{
					{
						Delta: &openai.Message{
							Content: openai.MessageContentString(repeatedText),
						},
					},
				},
			}
			body := octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionStreamChunk]{})
			body.SetParsed(chunk)
			chunks <- &octollm.StreamChunk{Body: body}
		}
	}()

	mockResp := &octollm.Response{
		Stream: octollm.NewStreamChan(chunks, func() {}),
	}

	mockEngine := &mockEngine{response: mockResp}

	detector := newDuplicationDetectorEngine(
		&DuplicationDetectorConfig{
			MinRepeatLen:    1,
			MaxRepeatLen:    5,
			RepeatThreshold: 50,
			DetectTimeout:   1 * time.Second,
		},
		"gpt-4",
		mockEngine,
	)

	httpReq, _ := http.NewRequestWithContext(context.Background(), "POST", "http://localhost/v1/chat/completions", nil)
	httpReq.URL, _ = url.Parse("http://localhost/v1/chat/completions")
	req := octollm.NewRequest(httpReq, octollm.APIFormatChatCompletions)
	req.Body = octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{})
	req.Body.SetParsed(&openai.ChatCompletionRequest{Model: "gpt-4"})

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Stream)

	// 消费所有 chunks
	count := 0
	for range result.Stream.Chan() {
		count++
	}
	assert.Equal(t, 60, count)

	// 给异步检测一些时间
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_Stream_NoRepetition(t *testing.T) {
	// 创建流式响应的 mock
	chunks := make(chan *octollm.StreamChunk, 10)

	// 模拟流式响应：发送不重复的文本
	go func() {
		defer close(chunks)
		texts := []string{"这", "是", "一", "段", "正", "常", "的", "文", "本"}
		for _, text := range texts {
			chunk := &openai.ChatCompletionStreamChunk{
				ID:    "test-stream-normal",
				Model: "gpt-4",
				Choices: []*openai.ChatCompletionStreamChoice{
					{
						Delta: &openai.Message{
							Content: openai.MessageContentString(text),
						},
					},
				},
			}
			body := octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionStreamChunk]{})
			body.SetParsed(chunk)
			chunks <- &octollm.StreamChunk{Body: body}
		}
	}()

	mockResp := &octollm.Response{
		Stream: octollm.NewStreamChan(chunks, func() {}),
	}

	mockEngine := &mockEngine{response: mockResp}

	detector := newDuplicationDetectorEngine(
		&DuplicationDetectorConfig{
			MinRepeatLen:    1,
			MaxRepeatLen:    5,
			RepeatThreshold: 50,
			DetectTimeout:   1 * time.Second,
		},
		"gpt-4",
		mockEngine,
	)

	httpReq, _ := http.NewRequestWithContext(context.Background(), "POST", "http://localhost/v1/chat/completions", nil)
	httpReq.URL, _ = url.Parse("http://localhost/v1/chat/completions")
	req := octollm.NewRequest(httpReq, octollm.APIFormatChatCompletions)
	req.Body = octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{})
	req.Body.SetParsed(&openai.ChatCompletionRequest{Model: "gpt-4"})

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Stream)

	// 消费所有 chunks
	count := 0
	for range result.Stream.Chan() {
		count++
	}
	assert.Equal(t, 9, count)

	// 给异步检测一些时间
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_EmptyResponse(t *testing.T) {
	resp := &openai.ChatCompletionResponse{
		ID:      "test-empty",
		Model:   "gpt-4",
		Choices: []*openai.ChatCompletionChoice{
			{
				Message: &openai.Message{
					Content: openai.MessageContentString(""),
				},
			},
		},
	}

	mockResp := &octollm.Response{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionResponse]{}),
	}
	mockResp.Body.SetParsed(resp)

	mockEngine := &mockEngine{response: mockResp}

	detector := newDuplicationDetectorEngine(
		&DuplicationDetectorConfig{
			MinRepeatLen:    1,
			MaxRepeatLen:    5,
			RepeatThreshold: 50,
			DetectTimeout:   1 * time.Second,
		},
		"gpt-4",
		mockEngine,
	)

	httpReq, _ := http.NewRequestWithContext(context.Background(), "POST", "http://localhost/v1/chat/completions", nil)
	httpReq.URL, _ = url.Parse("http://localhost/v1/chat/completions")
	req := octollm.NewRequest(httpReq, octollm.APIFormatChatCompletions)
	req.Body = octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{})
	req.Body.SetParsed(&openai.ChatCompletionRequest{Model: "gpt-4"})

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Body)

	// 给异步检测一些时间
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_Timeout(t *testing.T) {
	// 创建一个包含大量重复内容的响应，可能导致超时
	repeatedText := strings.Repeat("这是一个测试文本。", 1000)
	resp := &openai.ChatCompletionResponse{
		ID:      "test-timeout",
		Model:   "gpt-4",
		Choices: []*openai.ChatCompletionChoice{
			{
				Message: &openai.Message{
					Content: openai.MessageContentString(repeatedText),
				},
			},
		},
	}

	mockResp := &octollm.Response{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionResponse]{}),
	}
	mockResp.Body.SetParsed(resp)

	mockEngine := &mockEngine{response: mockResp}

	detector := newDuplicationDetectorEngine(
		&DuplicationDetectorConfig{
			MinRepeatLen:    1,
			MaxRepeatLen:    5,
			RepeatThreshold: 50,
			DetectTimeout:   1 * time.Nanosecond, // 极短的超时
		},
		"gpt-4",
		mockEngine,
	)

	httpReq, _ := http.NewRequestWithContext(context.Background(), "POST", "http://localhost/v1/chat/completions", nil)
	httpReq.URL, _ = url.Parse("http://localhost/v1/chat/completions")
	req := octollm.NewRequest(httpReq, octollm.APIFormatChatCompletions)
	req.Body = octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{})
	req.Body.SetParsed(&openai.ChatCompletionRequest{Model: "gpt-4"})

	result, err := detector.Process(req)
	require.NoError(t, err) // 超时不应该影响响应
	assert.NotNil(t, result)
	assert.NotNil(t, result.Body)

	// 给异步检测一些时间
	time.Sleep(100 * time.Millisecond)
}
