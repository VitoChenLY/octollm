package moderator

import (
	"context"
	"fmt"

	"github.com/infinigence/octollm/pkg/octollm"
	"github.com/infinigence/octollm/pkg/types/openai"
	"github.com/sirupsen/logrus"
)

type OpenAIAdapter struct {
	ReplacementTextForStreaming    string
	ReplacementTextForNonStreaming string
	ReplacementFinishReason        string
}

var _ TextModeratorAdapter = (*OpenAIAdapter)(nil)

func (a *OpenAIAdapter) ExtractTextFromBody(ctx context.Context, body *octollm.UnifiedBody) ([]rune, error) {
	parsed, err := body.Parsed()
	if err != nil {
		return nil, fmt.Errorf("parse body error: %w", err)
	}
	switch parsed := parsed.(type) {
	case *openai.ChatCompletionRequest:
		return a.extracTextFromRequest(ctx, parsed)
	case *openai.ChatCompletionResponse:
		return a.extracTextFromResponse(ctx, parsed)
	default:
		return nil, fmt.Errorf("unsupported body type: %T", parsed)
	}
}

func (a *OpenAIAdapter) extracTextFromRequest(ctx context.Context, body *openai.ChatCompletionRequest) ([]rune, error) {
	r := []rune{}
	for _, msg := range body.Messages {
		r = append(r, a.extractTextFromMessage(msg)...)
	}
	return r, nil
}

func (a *OpenAIAdapter) extracTextFromResponse(ctx context.Context, body *openai.ChatCompletionResponse) ([]rune, error) {
	if len(body.Choices) != 1 {
		return nil, fmt.Errorf("only support 1 choice, got %d", len(body.Choices))
	}

	choice := body.Choices[0]
	r := []rune{}

	// 处理非流式响应（Message）
	if choice.Message != nil {
		r = append(r, a.extractTextFromMessage(choice.Message)...)
	}

	// 处理流式响应（Delta）
	if choice.Delta != nil {
		r = append(r, a.extractTextFromMessage(choice.Delta)...)
	}

	return r, nil
}

// extractTextFromMessage 从 Message 中提取文本内容
func (a *OpenAIAdapter) extractTextFromMessage(msg *openai.Message) []rune {
	r := []rune{}

	// 提取 Content
	if msg.Content != nil {
		r = append(r, []rune(msg.Content.ExtractText())...)
	}

	// 提取 ReasoningContent
	if msg.ReasoningContent != nil {
		r = append(r, []rune(msg.ReasoningContent.ExtractText())...)
	}

	// 提取 ToolCalls
	for _, toolCall := range msg.ToolCalls {
		if toolCall.Function != nil {
			r = append(r, []rune(toolCall.Function.Arguments)...)
		}
	}

	return r
}

func (a *OpenAIAdapter) GetReplacementBody(ctx context.Context, body *octollm.UnifiedBody) *octollm.UnifiedBody {
	parsed, err := body.Parsed()
	if err != nil {
		logrus.WithContext(ctx).Debugf("parse body error: %s", err)
		return nil
	}
	switch parsed := parsed.(type) {
	case *openai.ChatCompletionResponse:
		r := a.getReplacementResponse(ctx, parsed)
		if r == nil {
			return nil
		}
		body.SetParsed(r)
		return body
	default:
		return nil
	}
}

func (a *OpenAIAdapter) getReplacementResponse(ctx context.Context, resp *openai.ChatCompletionResponse) *openai.ChatCompletionResponse {
	// 非流式响应
	if resp.Choices[0].Message != nil && a.ReplacementTextForNonStreaming != "" {
		r := &openai.ChatCompletionResponse{
			Id:      resp.Id,
			Object:  resp.Object,
			Created: resp.Created,
			Model:   resp.Model,
			Choices: []*openai.Choice{
				{
					Index: resp.Choices[0].Index,
					Message: &openai.Message{
						Role:    "assistant",
						Content: openai.MessageContentString(a.ReplacementTextForNonStreaming),
					},
					FinishReason: a.ReplacementFinishReason,
				},
			},
			Usage: resp.Usage,
		}
		return r
	}

	if resp.Choices[0].Delta != nil && a.ReplacementTextForStreaming != "" {
		r := &openai.ChatCompletionResponse{
			Id:      resp.Id,
			Object:  resp.Object,
			Created: resp.Created,
			Model:   resp.Model,
			Choices: []*openai.Choice{
				{
					Index: resp.Choices[0].Index,
					Delta: &openai.Message{
						Role:    "assistant",
						Content: openai.MessageContentString(a.ReplacementTextForStreaming),
					},
					FinishReason: a.ReplacementFinishReason,
				},
			},
		}
		return r
	}

	return nil
}
