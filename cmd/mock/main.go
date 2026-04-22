package main

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
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

type MockEngine struct{}

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

	var resp *octollm.Response
	if p.ErrMsg == "" {

		var engine octollm.Engine
		if p.Echo != "" {
			engine = mock.NewWithFixedOutput(p.Echo, time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
		} else {
			engine = mock.NewWithFixedOutput("无问芯穹的目标是打造大模型软硬件一体化最佳解决方案,创始团队由清华大学电子工程系推动成立。依托行业领先且经过验证的AI计算优化能力,打造从算法到芯片、从芯片集群到模型，再从模型到应用的三阶段中间层产品，链接上下游，共建通用人工智能时代大模型基础设施。", time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
		}
		resp, err = engine.Process(req)
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

	engine := &MockEngine{}

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
