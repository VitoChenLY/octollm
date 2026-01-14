package moderator

import (
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

	detector := NewDuplicationDetector(
		&OpenAIAdapter{},
		&DuplicationDetectorConfig{
			MinRepeatLen:    5,
			MaxRepeatLen:    50,
			RepeatThreshold: 3,
			DetectTimeout:   2 * time.Second,
		},
		mockEngine,
	)

	// 创建包含模型信息的请求
	reqBody := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []*openai.Message{
			{Role: "user", Content: openai.MessageContentString("test")},
		},
	}
	reqBodyUnified := octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{})
	reqBodyUnified.SetParsed(reqBody)

	req := &octollm.Request{
		Body: reqBodyUnified,
	}

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Body)

	// 等待一下让异步检测完成（仅用于测试）
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_NonStream_NoRepetition(t *testing.T) {
	// 创建一个没有重复内容的响应
	normalText := "This is a normal text without any repetition patterns."
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

	detector := NewDuplicationDetector(
		&OpenAIAdapter{},
		nil, // 使用默认配置
		mockEngine,
	)

	req := &octollm.Request{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{}),
	}

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// 等待一下让异步检测完成
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_Stream_WithRepetition(t *testing.T) {
	// 创建模拟的流式响应
	chunks := make(chan *octollm.StreamChunk, 5)
	repeatedText := "重复文本"

	for i := 0; i < 5; i++ {
		chunkResp := &openai.ChatCompletionStreamChunk{
			ID:    "chunk-123",
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
		body.SetParsed(chunkResp)

		chunks <- &octollm.StreamChunk{Body: body}
	}
	close(chunks)

	mockResp := &octollm.Response{
		Stream: octollm.NewStreamChan(chunks, nil),
	}

	mockEngine := &mockEngine{response: mockResp}

	detector := NewDuplicationDetector(
		&OpenAIAdapter{},
		&DuplicationDetectorConfig{
			MinRepeatLen:    4,
			MaxRepeatLen:    20,
			RepeatThreshold: 3,
			DetectTimeout:   2 * time.Second,
		},
		mockEngine,
	)

	req := &octollm.Request{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{}),
	}

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Stream)

	// 读取所有 chunks
	chunkCount := 0
	for range result.Stream.Chan() {
		chunkCount++
	}

	assert.Equal(t, 5, chunkCount, "Should receive all chunks")

	// 等待一下让异步检测完成
	time.Sleep(100 * time.Millisecond)
}

func TestDuplicationDetector_DetectTimeout(t *testing.T) {
	// 创建一个超长文本，可能导致检测超时
	longText := strings.Repeat("A", 100000)
	resp := &openai.ChatCompletionResponse{
		ID:      "test-timeout",
		Model:   "gpt-4",
		Choices: []*openai.ChatCompletionChoice{
			{
				Message: &openai.Message{
					Content: openai.MessageContentString(longText),
				},
			},
		},
	}

	mockResp := &octollm.Response{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionResponse]{}),
	}
	mockResp.Body.SetParsed(resp)

	mockEngine := &mockEngine{response: mockResp}

	detector := NewDuplicationDetector(
		&OpenAIAdapter{},
		&DuplicationDetectorConfig{
			MinRepeatLen:    10,
			MaxRepeatLen:    1000,
			RepeatThreshold: 3,
			DetectTimeout:   10 * time.Millisecond, // 非常短的超时
		},
		mockEngine,
	)

	req := &octollm.Request{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{}),
	}

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// 等待超时发生
	time.Sleep(50 * time.Millisecond)
}

func TestDuplicationDetector_DefaultConfig(t *testing.T) {
	config := DefaultDuplicationDetectorConfig()
	assert.Equal(t, 10, config.MinRepeatLen)
	assert.Equal(t, 100, config.MaxRepeatLen)
	assert.Equal(t, 3, config.RepeatThreshold)
	assert.Equal(t, 5*time.Second, config.DetectTimeout)
}

func TestDuplicationDetector_ErrorInExtraction(t *testing.T) {
	// 测试当文本提取失败时不应该影响正常响应
	mockResp := &octollm.Response{
		Body: octollm.NewBodyFromBytes([]byte("invalid"), &octollm.JSONParser[openai.ChatCompletionResponse]{}),
	}

	mockEngine := &mockEngine{response: mockResp}

	detector := NewDuplicationDetector(
		&OpenAIAdapter{},
		nil,
		mockEngine,
	)

	req := &octollm.Request{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{}),
	}

	result, err := detector.Process(req)
	require.NoError(t, err)
	assert.NotNil(t, result)
}

// BenchmarkDuplicationDetector 性能测试
func BenchmarkDuplicationDetector_NonStream(b *testing.B) {
	repeatedText := strings.Repeat("测试文本", 10)
	resp := &openai.ChatCompletionResponse{
		ID:      "bench-test",
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

	detector := NewDuplicationDetector(
		&OpenAIAdapter{},
		nil,
		mockEngine,
	)

	req := &octollm.Request{
		Body: octollm.NewBodyFromBytes([]byte{}, &octollm.JSONParser[openai.ChatCompletionRequest]{}),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := detector.Process(req)
		if err != nil {
			b.Fatal(err)
		}
	}
}
