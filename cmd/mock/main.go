package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/infinigence/octollm/pkg/engines/mock"
	ruleengine "github.com/infinigence/octollm/pkg/engines/rule-engine"
	"github.com/infinigence/octollm/pkg/exprenv"
	"github.com/infinigence/octollm/pkg/octollm"
)

type params struct {
	TTFT int    `json:"ttft"`
	TPOT int    `json:"tpot"`
	Echo string `json:"echo"`
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

	var engine octollm.Engine
	if p.Echo != "" {
		engine = mock.NewWithFixedOutput(p.Echo, time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
	} else {
		engine = mock.NewWithFixedOutput("无问芯穹的目标是打造大模型软硬件一体化最佳解决方案,创始团队由清华大学电子工程系推动成立。依托行业领先且经过验证的AI计算优化能力,打造从算法到芯片、从芯片集群到模型，再从模型到应用的三阶段中间层产品，链接上下游，共建通用人工智能时代大模型基础设施。", time.Duration(p.TTFT)*time.Millisecond, time.Duration(p.TPOT)*time.Millisecond)
	}
	resp, err := engine.Process(req)
	if err != nil {
		return nil, err
	}

	resp.Header.Set("x-mock-octollm-ttft", fmt.Sprintf("%dms", p.TTFT))
	resp.Header.Set("x-mock-octollm-tpot", fmt.Sprintf("%dms", p.TPOT))

	return resp, nil
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

	slog.Info("listening :8090")
	err := http.ListenAndServe(":8090", mux)
	slog.Error(fmt.Sprintf("server exited with error: %v", err))
}
