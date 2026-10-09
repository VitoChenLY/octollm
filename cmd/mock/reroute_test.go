package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/infinigence/octollm/pkg/octollm"
)

func TestMockEnginePrefillReroute(t *testing.T) {
	tests := []struct {
		name        string
		prefill     bool
		stream      bool
		config      string
		wantReroute bool
	}{
		{"P stream", true, true, `"prefill_reroute":{"enable":true},`, true},
		{"P direct request", true, true, "", true},
		{"P second attempt", true, true, `"prefill_reroute":{"enable":false},`, false},
		{"D stream", false, true, `"prefill_reroute":{"enable":true},`, false},
		{"P non-stream", true, false, `"prefill_reroute":{"enable":true},`, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"model":"mock","messages":[{"role":"user","content":"hello"}],` +
				test.config + `"mock_reroute":true,"stream":` + boolJSON(test.stream) + `,"ttft":1,"tpot":1,"echo":"ok"}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			octollm.ChatCompletionsHandler(&MockEngine{PrefillMode: test.prefill, FirstTokenOnly: test.prefill}).ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
			}
			hasReroute := strings.Contains(w.Body.String(), `"reroute":true`)
			if hasReroute != test.wantReroute {
				t.Fatalf("reroute signal = %v, want %v; body = %s", hasReroute, test.wantReroute, w.Body.String())
			}
			if test.wantReroute && !strings.Contains(w.Body.String(), "data: [DONE]") {
				t.Fatalf("missing stream terminator: %s", w.Body.String())
			}
		})
	}
}

func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
