package main

import (
	"context"
	"net/http"
	"time"

	"github.com/infinigence/octollm/pkg/octollm"
	"github.com/infinigence/octollm/pkg/types/openai"
)

// mockPrefillRerouteResponse emits the SGLang first-frame reroute signal.
// The caller closes the normal mock stream before replacing it with this one.
func mockPrefillRerouteResponse(ctx context.Context, ttft time.Duration) *octollm.Response {
	streamCtx, cancel := context.WithCancel(ctx)
	ch := make(chan *octollm.StreamChunk)
	go func() {
		defer close(ch)
		timer := time.NewTimer(ttft)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-streamCtx.Done():
			return
		}

		for _, body := range [][]byte{
			[]byte(`{"sglext":{"reroute":true,"reroute_reason":"LowCacheHitReroute","prompt_length":100,"cached_tokens":1}}`),
			[]byte("[DONE]"),
		} {
			select {
			case ch <- &octollm.StreamChunk{
				Body: octollm.NewBodyFromBytes(body, &octollm.JSONParser[openai.ChatCompletionStreamChunk]{}),
			}:
			case <-streamCtx.Done():
				return
			}
		}
	}()

	return octollm.NewStreamResponse(http.StatusOK,
		http.Header{"Content-Type": {"text/event-stream"}},
		octollm.NewStreamChan(ch, cancel))
}
