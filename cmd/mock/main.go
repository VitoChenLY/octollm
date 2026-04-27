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
	FirstTokenOnly bool // "P 模式"：只输出流里第一个内容 token，便于单测 TTFT
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
	slog.Info(fmt.Sprintf("Use TTFT: %dms (scale %g), TPOT: %dms (scale %g), firstTokenOnly=%v", p.TTFT, ttftScale, p.TPOT, tpotScale, e.FirstTokenOnly))
	var resp *octollm.Response
	if p.ErrMsg == "" {
		var m *mock.MockEndpoint
		if p.Echo != "" {
			m = mock.NewWithFixedOutput(p.Echo, time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
		} else {
			m = mock.NewWithFixedOutput("无问芯穹的目标是打造大模型软硬件一体化最佳解决方案,创始团队由清华大学电子工程系推动成立。依托行业领先且经过验证的AI计算优化能力,打造从算法到芯片、从芯片集群到模型，再从模型到应用的三阶段中间层产品，链接上下游，共建通用人工智能时代大模型基础设施。", time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
		}
		m.FirstTokenOnly = e.FirstTokenOnly
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

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	port := flag.String("port", "", "mock server listen port (e.g. 8090)")
	ttftScale := flag.Float64("ttft-scale", 1, "scale factor for request JSON ttft (ms) after defaults, must be >0 (default: no scaling)")
	tpotScale := flag.Float64("tpot-scale", 1, "scale factor for request JSON tpot (ms) after defaults, must be >0 (default: no scaling)")
	pMode := flag.Bool("p", false, "P mode: only emit the first content token in stream (and one rune in non-stream); isolates TTFT from long TPOT tail")
	flag.Parse()

	listenPort := "30000"
	if envPort := os.Getenv("MOCK_SERVER_PORT"); envPort != "" {
		listenPort = envPort
	}
	if *port != "" {
		listenPort = *port
	}

	exprenv.RegisterDefaultExtractor("promptTextLen", &ruleengine.PromptTextLenExtractor{})
	exprenv.RegisterDefaultExtractor("prefix20", &ruleengine.PrefixHashExtractor{Length: 20})
	exprenv.RegisterDefaultExtractor("suffix20", &ruleengine.SuffixHashExtractor{Length: 20})
	exprenv.RegisterDefaultExtractor("message5Hash", &ruleengine.Message5HashExtractor{})
	exprenv.RegisterDefaultExtractor("message5HashArray", &ruleengine.Message5HashArrayExtractor{})

	engine := &MockEngine{
		TTFTScale:      *ttftScale,
		TPOTScale:      *tpotScale,
		FirstTokenOnly: *pMode,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/v1/chat/completions", gzipMiddleware(octollm.ChatCompletionsHandler(engine)))

	addr := ":" + listenPort
	slog.Info(fmt.Sprintf("listening %s", addr))
	err := http.ListenAndServe(addr, mux)
	slog.Error(fmt.Sprintf("server exited with error: %v", err))
}
