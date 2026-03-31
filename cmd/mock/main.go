package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/infinigence/octollm/pkg/engines/mock"
	ruleengine "github.com/infinigence/octollm/pkg/engines/rule-engine"
	"github.com/infinigence/octollm/pkg/exprenv"
	"github.com/infinigence/octollm/pkg/octollm"
)

type params struct {
	TTFT    int    `json:"ttft"`
	TPOT    int    `json:"tpot"`
	SvcName int    `json:"svc_name"`
	Echo    string `json:"echo"`
}

type MockEngine struct{}

var (
	svc1 atomic.Int32
	svc2 atomic.Int32
	svc3 atomic.Int32
)

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

	var chosen *atomic.Int32
	switch p.SvcName {
	case 1:
		chosen = &svc1
	case 2:
		chosen = &svc2
	case 3:
		chosen = &svc3
	}
	if chosen != nil {
		chosen.Add(1)
		defer chosen.Add(-1)
	}

	va1 := svc1.Load()
	va2 := svc2.Load()
	va3 := svc3.Load()

	slog.Info(fmt.Sprintf("current svc count: svc1=%d, svc2=%d, svc3=%d", va1, va2, va3))

	var engine octollm.Engine
	if p.Echo != "" {
		engine = mock.NewWithFixedOutput(p.Echo, time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
	} else {
		engine = mock.NewWithFixedOutput("无问芯穹的目标是打造大模型软硬件一体化最佳解决方案,创始团队由清华大学电子工程系推动成立。依托行业领先且经过验证的AI计算优化能力,打造从算法到芯片、从芯片集群到模型，再从模型到应用的三阶段中间层产品，链接上下游，共建通用人工智能时代大模型基础设施。", time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
	}
	return engine.Process(req)
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	exprenv.RegisterDefaultExtractor("promptTextLen", &ruleengine.PromptTextLenExtractor{})
	exprenv.RegisterDefaultExtractor("prefix20", &ruleengine.PrefixHashExtractor{Length: 20})
	exprenv.RegisterDefaultExtractor("suffix20", &ruleengine.SuffixHashExtractor{Length: 20})
	exprenv.RegisterDefaultExtractor("message5Hash", &ruleengine.Message5HashExtractor{})
	exprenv.RegisterDefaultExtractor("message5HashArray", &ruleengine.Message5HashArrayExtractor{})

	engine := &MockEngine{}

	mux := http.NewServeMux()
	mux.Handle("/v1/chat/completions", octollm.ChatCompletionsHandler(engine))
	mux.Handle("/v1/completions", octollm.CompletionsHandler(engine))
	mux.Handle("/v1/messages", octollm.MessagesHandler(engine))

	slog.Info("listening :8090")
	err := http.ListenAndServe(":8090", mux)
	slog.Error(fmt.Sprintf("server exited with error: %v", err))
}
