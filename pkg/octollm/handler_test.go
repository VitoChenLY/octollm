package octollm

import (
	"testing"
)

func TestExtractVertexModelFromURL(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantModel string
		wantStream bool
	}{
		{
			name:       "stream mode with colon",
			path:       "/v1/models/gemini-3-pro:streamGenerateContent",
			wantModel:  "gemini-3-pro",
			wantStream: true,
		},
		{
			name:       "non-stream mode with colon",
			path:       "/v1/models/gemini-3-pro:generateContent",
			wantModel:  "gemini-3-pro",
			wantStream: false,
		},
		{
			name:       "stream mode with model name containing hyphens",
			path:       "/v1/models/gemini-3-pro-image-preview:streamGenerateContent",
			wantModel:  "gemini-3-pro-image-preview",
			wantStream: true,
		},
		{
			name:       "non-stream mode with model name containing hyphens",
			path:       "/v1/models/gemini-3-pro-image-preview:generateContent",
			wantModel:  "gemini-3-pro-image-preview",
			wantStream: false,
		},
		{
			name:       "stream mode with simple model name",
			path:       "/v1/models/gemini:streamGenerateContent",
			wantModel:  "gemini",
			wantStream: true,
		},
		{
			name:       "non-stream mode with simple model name",
			path:       "/v1/models/gemini:generateContent",
			wantModel:  "gemini",
			wantStream: false,
		},
		{
			name:       "path without /models/",
			path:       "/v1/chat/completions",
			wantModel:  "",
			wantStream: false,
		},
		{
			name:       "path without colon",
			path:       "/v1/models/gemini-3-pro",
			wantModel:  "",
			wantStream: false,
		},
		{
			name:       "path with multiple /models/",
			path:       "/v1/models/gemini-3-pro/models/test:generateContent",
			wantModel:  "test",
			wantStream: false,
		},
		{
			name:       "empty path",
			path:       "",
			wantModel:  "",
			wantStream: false,
		},
		{
			name:       "path with only /models/",
			path:       "/v1/models/",
			wantModel:  "",
			wantStream: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotModel, gotStream := extractVertexModelFromURL(tt.path)
			if gotModel != tt.wantModel {
				t.Errorf("extractVertexModelFromURL() modelName = %v, want %v", gotModel, tt.wantModel)
			}
			if gotStream != tt.wantStream {
				t.Errorf("extractVertexModelFromURL() isStream = %v, want %v", gotStream, tt.wantStream)
			}
		})
	}
}
