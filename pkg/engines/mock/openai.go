package mock

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/infinigence/octollm/pkg/octollm"
	"github.com/infinigence/octollm/pkg/types/openai"
)

type MockEndpoint struct {
	OutputString string //  the output string to return
	TTFT         time.Duration
	TPOT         time.Duration
	// FirstTokenOnly, if true, only emits the first rune of OutputString (one stream chunk of content, then finish).
	// Non-stream still returns a single rune. Useful to isolate TTFT from TPOT in benchmarks.
	FirstTokenOnly bool
	// DecodeMode adds cumulative token_ids (Unicode codepoints) to each stream chunk at the choice level.
	DecodeMode bool
}

type DDelta struct {
	Role             string                `json:"role,omitempty"`
	Content          openai.MessageContent `json:"content,omitempty"`
	ReasoningContent openai.MessageContent `json:"reasoning_content,omitempty"`
	TokenIDs         []int                 `json:"token_ids,omitempty"`
}

type DStreamChoice struct {
	FinishReason string   `json:"finish_reason,omitempty"`
	Index        int      `json:"index"`
	Delta        *DDelta  `json:"delta"`
}

type DStreamChunk struct {
	ID      string          `json:"id"`
	Created int             `json:"created"`
	Object  string          `json:"object,omitempty"`
	Model   string          `json:"model"`
	Choices []*DStreamChoice `json:"choices"`
	Usage   *openai.Usage   `json:"usage,omitempty"`
}

var _ octollm.Engine = (*MockEndpoint)(nil)

func NewWithFixedOutput(outputString string, ttft, tpot time.Duration) *MockEndpoint {
	return &MockEndpoint{
		OutputString: outputString,
		TTFT:         ttft,
		TPOT:         tpot,
	}
}

func (e *MockEndpoint) Process(req *octollm.Request) (*octollm.Response, error) {
	reqBody, err := req.Body.Parsed()
	if err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	switch v := reqBody.(type) {
	case *openai.ChatCompletionRequest:
		if v.Stream != nil && *v.Stream {
			if e.DecodeMode {
				return e.openAIDStreamResponse(req, v)
			}
			return e.openAIStreamResponse(req, v)
		}
		return e.openAINonStreamResponse(req, v)
	default:
		return nil, fmt.Errorf("unexpected request body type: %T", reqBody)
	}
}

func (e *MockEndpoint) openAINonStreamResponse(req *octollm.Request, v *openai.ChatCompletionRequest) (*octollm.Response, error) {
	rOutput := []rune(e.OutputString)
	finishReason := "stop"
	if v.MaxTokens != nil && len(rOutput) > *v.MaxTokens {
		rOutput = rOutput[:*v.MaxTokens]
		finishReason = "length"
	}
	if e.FirstTokenOnly && len(rOutput) > 0 {
		rOutput = rOutput[:1]
	}
	time.Sleep(e.TTFT + e.TPOT*time.Duration(len(rOutput)))
	bodyVal := &openai.ChatCompletionResponse{
		ID:      "mock-id",
		Object:  "chat.completion",
		Created: int(time.Now().Unix()),
		Model:   v.Model,
		Choices: []*openai.ChatCompletionChoice{
			{
				Index: 0,
				Message: &openai.Message{
					Role:    "assistant",
					Content: openai.MessageContentString(string(rOutput)),
				},
				FinishReason: finishReason,
			},
		},
		Usage: &openai.Usage{
			PromptTokens:     1,
			CompletionTokens: 100,
			TotalTokens:      101,
		},
	}
	body := octollm.NewBodyFromParsed(bodyVal, &octollm.JSONParser[openai.ChatCompletionResponse]{})
	resp := octollm.NewNonStreamResponse(200, http.Header{"Content-Type": {"application/json"}}, body)
	return resp, nil
}

func (e *MockEndpoint) openAIStreamResponse(req *octollm.Request, v *openai.ChatCompletionRequest) (*octollm.Response, error) {
	rOutput := []rune(e.OutputString)
	finishReason := "stop"
	if v.MaxTokens != nil && len(rOutput) > *v.MaxTokens {
		rOutput = rOutput[:*v.MaxTokens]
		finishReason = "length"
	}
	if e.FirstTokenOnly && len(rOutput) > 0 {
		rOutput = rOutput[:1]
	}

	ch := make(chan *octollm.StreamChunk)
	ctx, cancel := context.WithCancel(req.Context())

	go func() {
		defer close(ch)
		time.Sleep(e.TTFT)

		for _, c := range rOutput {
			bodyVal := &openai.ChatCompletionStreamChunk{
				ID:      "mock-id",
				Object:  "chat.completion.chunk",
				Created: int(time.Now().Unix()),
				Model:   v.Model,
				Choices: []*openai.ChatCompletionStreamChoice{
					{
						Index: 0,
						Delta: &openai.Message{
							Role:    "assistant",
							Content: openai.MessageContentString(string(c)),
						},
					},
				},
			}
			select {
			case ch <- &octollm.StreamChunk{
				Body: octollm.NewBodyFromParsed(bodyVal, &octollm.JSONParser[openai.ChatCompletionStreamChunk]{}),
			}:
			case <-ctx.Done():
				slog.InfoContext(ctx, fmt.Sprintf("[http-endpoint] context canceled during stream response: %v", ctx.Err()))
				return
			}
			time.Sleep(e.TPOT)
		}
		bodyVal := &openai.ChatCompletionStreamChunk{
			ID:      "mock-id",
			Object:  "chat.completion.chunk",
			Created: int(time.Now().Unix()),
			Model:   v.Model,
			Choices: []*openai.ChatCompletionStreamChoice{
				{
					Index:        0,
					FinishReason: finishReason,
					Delta: &openai.Message{
						Content: openai.MessageContentString(""),
					},
				},
			},
			Usage: &openai.Usage{
				PromptTokens:     1,
				CompletionTokens: 100,
				TotalTokens:      101,
			},
		}
		select {
		case ch <- &octollm.StreamChunk{
			Body: octollm.NewBodyFromParsed(bodyVal, &octollm.JSONParser[openai.ChatCompletionStreamChunk]{}),
		}:
		case <-ctx.Done():
			slog.InfoContext(ctx, fmt.Sprintf("[http-endpoint] context canceled during stream response: %v", ctx.Err()))
			return
		}

		select {
		case ch <- &octollm.StreamChunk{
			Body: octollm.NewBodyFromBytes([]byte("[DONE]"), &octollm.JSONParser[openai.ChatCompletionStreamChunk]{}),
		}:
		case <-ctx.Done():
			slog.InfoContext(ctx, fmt.Sprintf("[http-endpoint] context canceled during stream response: %v", ctx.Err()))
			return
		}
	}()

	streamChan := octollm.NewStreamChan(ch, cancel)
	resp := octollm.NewStreamResponse(200, http.Header{"Content-Type": {"text/event-stream"}}, streamChan)
	return resp, nil
}

func (e *MockEndpoint) openAIDStreamResponse(req *octollm.Request, v *openai.ChatCompletionRequest) (*octollm.Response, error) {
	rOutput := []rune(e.OutputString)
	finishReason := "stop"
	if v.MaxTokens != nil && len(rOutput) > *v.MaxTokens {
		rOutput = rOutput[:*v.MaxTokens]
		finishReason = "length"
	}

	ch := make(chan *octollm.StreamChunk)
	ctx, cancel := context.WithCancel(req.Context())

	go func() {
		defer close(ch)
		time.Sleep(e.TTFT)

		var cumTokenIDs []int
		for _, c := range rOutput {
			cumTokenIDs = append(cumTokenIDs, int(c))
			ids := make([]int, len(cumTokenIDs))
			copy(ids, cumTokenIDs)

			bodyVal := &DStreamChunk{
				ID:      "mock-id",
				Object:  "chat.completion.chunk",
				Created: int(time.Now().Unix()),
				Model:   v.Model,
				Choices: []*DStreamChoice{
					{
						Index: 0,
						Delta: &DDelta{
							Role:     "assistant",
							Content:  openai.MessageContentString(string(c)),
							TokenIDs: ids,
						},
					},
				},
			}
			select {
			case ch <- &octollm.StreamChunk{
				Body: octollm.NewBodyFromParsed(bodyVal, &octollm.JSONParser[DStreamChunk]{}),
			}:
			case <-ctx.Done():
				return
			}
			time.Sleep(e.TPOT)
		}

		finalIDs := make([]int, len(cumTokenIDs))
		copy(finalIDs, cumTokenIDs)
		bodyVal := &DStreamChunk{
			ID:      "mock-id",
			Object:  "chat.completion.chunk",
			Created: int(time.Now().Unix()),
			Model:   v.Model,
			Choices: []*DStreamChoice{
				{
					Index:        0,
					FinishReason: finishReason,
					Delta: &DDelta{
						Content:  openai.MessageContentString(""),
						TokenIDs: finalIDs,
					},
				},
			},
			Usage: &openai.Usage{
				PromptTokens:     1,
				CompletionTokens: len(cumTokenIDs),
				TotalTokens:      1 + len(cumTokenIDs),
			},
		}
		select {
		case ch <- &octollm.StreamChunk{
			Body: octollm.NewBodyFromParsed(bodyVal, &octollm.JSONParser[DStreamChunk]{}),
		}:
		case <-ctx.Done():
			return
		}

		select {
		case ch <- &octollm.StreamChunk{
			Body: octollm.NewBodyFromBytes([]byte("[DONE]"), &octollm.JSONParser[DStreamChunk]{}),
		}:
		case <-ctx.Done():
			return
		}
	}()

	streamChan := octollm.NewStreamChan(ch, cancel)
	resp := octollm.NewStreamResponse(200, http.Header{"Content-Type": {"text/event-stream"}}, streamChan)
	return resp, nil
}
