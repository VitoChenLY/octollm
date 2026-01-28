package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/infinigence/octollm/pkg/octollm"
	"github.com/infinigence/octollm/pkg/types/anthropic"
	"github.com/infinigence/octollm/pkg/types/openai"
	"github.com/infinigence/octollm/pkg/types/rerank"
	"github.com/infinigence/octollm/pkg/types/vertex"
)

type GeneralEndpoint struct {
	*HTTPEndpoint
}

// GeneralEndpoint implements octollm.Endpoint
var _ octollm.Engine = (*GeneralEndpoint)(nil)

type GeneralEndpointConfig struct {
	BaseURL   string
	Endpoints map[octollm.APIFormat]string
	APIKey    string

	AnthropicAPIKeyAsBearer bool
	// GoogleApiKeyAsBearer specifies the authentication method for Google Vertex AI

	GoogleApiKeyAsBearer bool
}

var DefaultURLPathChatCompletions = "/v1/chat/completions"
var DefaultURLPathCompletions = "/v1/completions"
var DefaultURLPathClaudeMessages = "/v1/messages"
var DefaultURLPathVertex = "/models/{model}" // Base path for Vertex AI, action will be appended based on stream mode
var DefaultURLPathEmbeddings = "/v1/embeddings"
var DefaultURLPathRerank = "/v1/rerank"

func NewGeneralEndpoint(conf GeneralEndpointConfig) *GeneralEndpoint {
	apiKey := conf.APIKey
	if apiKey == "" {
		// read from env
		apiKey = os.Getenv("OCTOLLM_API_KEY")
	}

	httpEndpoint := NewHTTPEndpoint().
		WithURLGetter(func(req *octollm.Request) (string, error) {
			endpoint, ok := conf.Endpoints[req.Format]
			if !ok {
				return "", fmt.Errorf("invalid format: %s", req.Format)
			}
			if endpoint == "" {
				switch req.Format {
				case octollm.APIFormatClaudeMessages:
					endpoint = DefaultURLPathClaudeMessages
				case octollm.APIFormatChatCompletions:
					endpoint = DefaultURLPathChatCompletions
				case octollm.APIFormatCompletions:
					endpoint = DefaultURLPathCompletions
				case octollm.APIFormatGoogleGenerateContent:
					endpoint = DefaultURLPathVertex
				case octollm.APIFormatEmbeddings:
					endpoint = DefaultURLPathEmbeddings
				case octollm.APIFormatRerank:
					endpoint = DefaultURLPathRerank
				default:
					return "", fmt.Errorf("invalid format: %s", req.Format)
				}
			}

			// For Vertex AI, replace {model} placeholder and append action based on stream mode
			if req.Format == octollm.APIFormatGoogleGenerateContent {
				// Get model name from context
				modelName, ok := req.Context().Value(octollm.ContextKeyModelName).(string)
				if !ok || modelName == "" {
					return "", fmt.Errorf("model name not found in Vertex AI request context")
				}
				
				// Replace {model} placeholder with actual model name
				endpoint = strings.ReplaceAll(endpoint, "{model}", modelName)
				
				// Get stream mode from context
				isStream, _ := req.Context().Value(octollm.ContextKeyStreamMode).(bool)
				
				// Remove any existing action (for backward compatibility with old configs)
				endpoint = strings.TrimSuffix(endpoint, ":generateContent")
				endpoint = strings.TrimSuffix(endpoint, ":streamGenerateContent")
				
				// Append the correct action based on stream mode
				if isStream {
					endpoint = endpoint + ":streamGenerateContent"
				} else {
					endpoint = endpoint + ":generateContent"
				}
			}

			return conf.BaseURL + endpoint, nil
		}).
		WithParser(
			func(req *octollm.Request) octollm.Parser {
				switch req.Format {
				case octollm.APIFormatChatCompletions:
					return &octollm.JSONParser[openai.ChatCompletionRequest]{}
				case octollm.APIFormatClaudeMessages:
					return &octollm.JSONParser[anthropic.ClaudeMessagesResponse]{}
				case octollm.APIFormatCompletions:
					return &octollm.JSONParser[openai.CompletionResponse]{}
				case octollm.APIFormatGoogleGenerateContent:
					return &octollm.JSONParser[vertex.GenerateContentResponse]{}
				case octollm.APIFormatEmbeddings:
					return &octollm.JSONParser[openai.EmbeddingResponse]{}
				case octollm.APIFormatRerank:
					return &octollm.JSONParser[rerank.RerankResponse]{}
				default:
					return &octollm.JSONParser[json.RawMessage]{}
				}
			},
			func(req *octollm.Request) (octollm.Parser, StreamingType) {
				switch req.Format {
				case octollm.APIFormatChatCompletions:
					return &octollm.JSONParser[openai.ChatCompletionStreamChunk]{}, StreamingTypeSSE
				case octollm.APIFormatCompletions:
					return &octollm.JSONParser[openai.CompletionStreamChunk]{}, StreamingTypeSSE
				case octollm.APIFormatClaudeMessages:
					return &octollm.JSONParser[anthropic.ClaudeMessagesStreamEvent]{}, StreamingTypeSSE
				case octollm.APIFormatGoogleGenerateContent:
					return &octollm.JSONParser[vertex.StreamGenerateContentResponse]{}, StreamingTypeJSON
				default:
					return &octollm.JSONParser[json.RawMessage]{}, StreamingTypeSSE
				}
			},
		)

	if apiKey != "" {
		httpEndpoint = httpEndpoint.WithRequestModifier(func(req *octollm.Request, httpReq *http.Request) *http.Request {
			// Handle different authentication methods
			if req.Format == octollm.APIFormatClaudeMessages && !conf.AnthropicAPIKeyAsBearer {
				// Claude with x-api-key header
				httpReq.Header.Set("x-api-key", apiKey)
			} else if req.Format == octollm.APIFormatGoogleGenerateContent && !conf.GoogleApiKeyAsBearer {
				// Google Vertex AI with API Key
				httpReq.Header.Set("x-goog-api-key", apiKey)
			} else {
				// Default: Bearer token for OpenAI, Google OAuth, and others
				httpReq.Header.Set("Authorization", "Bearer "+apiKey)
			}
			return httpReq
		})
	}

	return &GeneralEndpoint{
		HTTPEndpoint: httpEndpoint,
	}
}
