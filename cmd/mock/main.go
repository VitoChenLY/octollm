package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/infinigence/octollm/pkg/engines/mock"
	ruleengine "github.com/infinigence/octollm/pkg/engines/rule-engine"
	"github.com/infinigence/octollm/pkg/errutils"
	"github.com/infinigence/octollm/pkg/exprenv"
	"github.com/infinigence/octollm/pkg/octollm"
	"github.com/infinigence/octollm/pkg/types/openai"
)

type params struct {
	TTFT       int    `json:"ttft"`
	TPOT       int    `json:"tpot"`
	Echo       string `json:"echo"`
	StatusCode int    `json:"err_status_code"`
	ErrMsg     string `json:"err_msg"`
}

// MockEngine is the HTTP-injected engine; scale factors apply to JSON body ttft/tpot (ms) after defaults.
type MockEngine struct {
	TTFTScale      float64
	TPOTScale      float64
	FirstTokenOnly bool
	DecodeMode     bool
}

func (e *MockEngine) Process(req *octollm.Request) (*octollm.Response, error) {
	buffer, err := req.Body.Bytes()
	if err != nil {
		return nil, err
	}
	var p params
	json.Unmarshal(buffer, &p)
	if p.TTFT == 0 {
		p.TTFT = 100
	}
	if p.TPOT == 0 {
		p.TPOT = 100
	}
	if p.StatusCode == 0 {
		p.StatusCode = 400
	}
	ttftScale, tpotScale := e.TTFTScale, e.TPOTScale
	if ttftScale <= 0 {
		ttftScale = 1
	}
	if tpotScale <= 0 {
		tpotScale = 1
	}
	p.TTFT = int(math.Max(1, float64(p.TTFT)*ttftScale))
	p.TPOT = int(math.Max(1, float64(p.TPOT)*tpotScale))
	slog.Info(fmt.Sprintf("Use TTFT: %dms (scale %g), TPOT: %dms (scale %g), firstTokenOnly=%v, decodeMode=%v", p.TTFT, ttftScale, p.TPOT, tpotScale, e.FirstTokenOnly, e.DecodeMode))
	var resp *octollm.Response
	if p.ErrMsg == "" {
		var m *mock.MockEndpoint
		if p.Echo != "" {
			m = mock.NewWithFixedOutput(p.Echo, time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
		} else {
			m = mock.NewWithFixedOutput("无问芯穹的目标是打造大模型软硬件一体化最佳解决方案,创始团队由清华大学电子工程系推动成立。依托行业领先且经过验证的AI计算优化能力,打造从算法到芯片、从芯片集群到模型，再从模型到应用的三阶段中间层产品，链接上下游，共建通用人工智能时代大模型基础设施。", time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
		}
		m.FirstTokenOnly = e.FirstTokenOnly
		m.DecodeMode = e.DecodeMode
		resp, err = m.Process(req)
		if err != nil {
			return nil, err
		}
	} else {
		resp = &octollm.Response{
			StatusCode: p.StatusCode,
			Body:       octollm.NewBodyFromBytes([]byte(p.ErrMsg), &octollm.JSONParser[openai.ChatCompletionResponse]{}),
			Header:     http.Header{},
		}
		err := errutils.NewHandlerError(
			errors.New(p.ErrMsg),
			p.StatusCode,
			p.ErrMsg,
		)
		return resp, err
	}

	resp.Header.Set("x-mock-octollm-ttft", fmt.Sprintf("%dms", p.TTFT))
	resp.Header.Set("x-mock-octollm-tpot", fmt.Sprintf("%dms", p.TPOT))

	return resp, nil
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, `{"error": "invalid gzip"}`, http.StatusBadRequest)
				return
			}
			defer gz.Close()

			r.Body = gz
			r.Header.Del("Content-Encoding")
			r.Header.Del("Content-Length")
			r.ContentLength = -1
		}
		next.ServeHTTP(w, r)
	})
}

type RDHandler struct {
	HandoverAfter int

	appendCountsMu sync.Mutex
	appendCounts   map[int64]int

	handoverMu    sync.Mutex
	handoverChans map[int64]chan struct{}
}

func (h *RDHandler) getHandoverChan(room int64) <-chan struct{} {
	h.handoverMu.Lock()
	defer h.handoverMu.Unlock()
	ch, ok := h.handoverChans[room]
	if !ok {
		ch = make(chan struct{})
		h.handoverChans[room] = ch
	}
	return ch
}

func (h *RDHandler) signalHandover(room int64) {
	h.handoverMu.Lock()
	defer h.handoverMu.Unlock()
	ch, ok := h.handoverChans[room]
	if ok {
		close(ch)
		delete(h.handoverChans, room)
	}
}

type RDEngine struct {
	inner   *MockEngine
	handler *RDHandler
}

func (e *RDEngine) Process(req *octollm.Request) (*octollm.Response, error) {
	buffer, _ := req.Body.Bytes()
	var extra struct {
		BootstrapRoom int64 `json:"bootstrap_room"`
	}
	json.Unmarshal(buffer, &extra)

	handoverCh := e.handler.getHandoverChan(extra.BootstrapRoom)

	innerResp, err := e.inner.Process(req)
	if err != nil {
		return nil, err
	}
	if innerResp.Stream == nil {
		return innerResp, nil
	}

	innerStream := innerResp.Stream
	ch := make(chan *octollm.StreamChunk)

	go func() {
		defer close(ch)
		select {
		case <-handoverCh:
		case <-req.Context().Done():
			innerStream.Close()
			return
		}
		for chunk := range innerStream.Chan() {
			select {
			case ch <- chunk:
			case <-req.Context().Done():
				innerStream.Close()
				return
			}
		}
	}()

	newStream := octollm.NewStreamChan(ch, innerStream.Close)
	return octollm.NewStreamResponse(innerResp.StatusCode, innerResp.Header, newStream), nil
}

type appendTokensRequest struct {
	BootstrapRoom int64 `json:"bootstrap_room"`
	TokenIDs      []int `json:"token_ids"`
	LastIdx       int   `json:"last_idx"`
}

type appendTokensResponse struct {
	Handover bool `json:"handover"`
	LastIdx  int  `json:"last_idx"`
}

func (h *RDHandler) HandleAppendTokens(w http.ResponseWriter, r *http.Request) {
	var req appendTokensRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	h.appendCountsMu.Lock()
	h.appendCounts[req.BootstrapRoom]++
	count := h.appendCounts[req.BootstrapRoom]
	handover := count >= h.HandoverAfter
	if handover {
		delete(h.appendCounts, req.BootstrapRoom)
	}
	h.appendCountsMu.Unlock()

	if handover {
		h.signalHandover(req.BootstrapRoom)
	}

	slog.Info(fmt.Sprintf("append_tokens: bootstrap_room=%d, count=%d, handover=%v", req.BootstrapRoom, count, handover))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(appendTokensResponse{Handover: handover, LastIdx: req.LastIdx})
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	port := flag.String("port", "", "mock server listen port (e.g. 8090)")
	ttftScale := flag.Float64("ttft-scale", 1, "scale factor for request JSON ttft (ms) after defaults, must be >0 (default: no scaling)")
	tpotScale := flag.Float64("tpot-scale", 1, "scale factor for request JSON tpot (ms) after defaults, must be >0 (default: no scaling)")
	pMode := flag.Bool("p", false, "P mode: only emit the first content token in stream (and one rune in non-stream); isolates TTFT from long TPOT tail")
	role := flag.String("role", "", "server role: P (prefill), D (decode), RD (router/dispatcher)")
	rdHandoverAfter := flag.Int("rd-handover-after", 3, "RD mode: return handover=true after this many append_tokens requests per bootstrap_room")
	flag.Parse()

	listenPort := "30000"
	if envPort := os.Getenv("MOCK_SERVER_PORT"); envPort != "" {
		listenPort = envPort
	}
	if *port != "" {
		listenPort = *port
	}

	normalizedRole := strings.ToUpper(*role)

	exprenv.RegisterDefaultExtractor("promptTextLen", &ruleengine.PromptTextLenExtractor{})
	exprenv.RegisterDefaultExtractor("prefix20", &ruleengine.PrefixHashExtractor{Length: 20})
	exprenv.RegisterDefaultExtractor("suffix20", &ruleengine.SuffixHashExtractor{Length: 20})
	exprenv.RegisterDefaultExtractor("message5Hash", &ruleengine.Message5HashExtractor{})
	exprenv.RegisterDefaultExtractor("message5HashArray", &ruleengine.Message5HashArrayExtractor{})

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	switch normalizedRole {
	case "RD":
		rdHandler := &RDHandler{
			HandoverAfter:  *rdHandoverAfter,
			appendCountsMu: sync.Mutex{},
			appendCounts:   make(map[int64]int),
			handoverMu:     sync.Mutex{},
			handoverChans:  make(map[int64]chan struct{}),
		}
		rdEngine := &RDEngine{
			inner:   &MockEngine{TTFTScale: *ttftScale, TPOTScale: *tpotScale},
			handler: rdHandler,
		}
		mux.Handle("/v1/chat/completions", gzipMiddleware(octollm.ChatCompletionsHandler(rdEngine)))
		mux.HandleFunc("/append_tokens", rdHandler.HandleAppendTokens)
		slog.Info(fmt.Sprintf("RD mode: handover-after=%d", *rdHandoverAfter))

	default:
		engine := &MockEngine{
			TTFTScale:      *ttftScale,
			TPOTScale:      *tpotScale,
			FirstTokenOnly: *pMode || normalizedRole == "P",
			DecodeMode:     normalizedRole == "D",
		}
		mux.Handle("/v1/chat/completions", gzipMiddleware(octollm.ChatCompletionsHandler(engine)))
		if normalizedRole != "" {
			slog.Info(fmt.Sprintf("Role: %s", normalizedRole))
		}
	}

	addr := ":" + listenPort
	slog.Info(fmt.Sprintf("listening %s", addr))
	err := http.ListenAndServe(addr, mux)
	slog.Error(fmt.Sprintf("server exited with error: %v", err))
}
