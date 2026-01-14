package moderator

import (
	"context"
	"time"

	"github.com/infinigence/octollm/pkg/octollm"
	"github.com/infinigence/octollm/pkg/types/anthropic"
	"github.com/infinigence/octollm/pkg/types/openai"
	"github.com/infinigence/octollm/pkg/utils"
	"github.com/sirupsen/logrus"
)

// DuplicationDetectorConfig 重复检测配置
type DuplicationDetectorConfig struct {
	// 最小重复 pattern 长度（字符数）
	MinRepeatLen int
	// 最大重复 pattern 长度（字符数）
	MaxRepeatLen int
	// 重复次数阈值，达到此阈值才认为是重复
	RepeatThreshold int
	// 检测超时时间
	DetectTimeout time.Duration
}

// DefaultDuplicationDetectorConfig 返回默认配置
func DefaultDuplicationDetectorConfig() *DuplicationDetectorConfig {
	return &DuplicationDetectorConfig{
		MinRepeatLen:    1,  // 最小 1 个字符
		MaxRepeatLen:    5,  // 最大 5 个字符
		RepeatThreshold: 50, // 重复 3 次以上
		DetectTimeout:   1 * time.Second,
	}
}

// DuplicationDetectorEngine 重复内容检测引擎
type DuplicationDetectorEngine struct {
	next octollm.Engine

	adapter TextModeratorAdapter
	config  *DuplicationDetectorConfig
}

var _ octollm.Engine = (*DuplicationDetectorEngine)(nil)

// NewDuplicationDetector 创建重复检测引擎
func NewDuplicationDetector(
	adapter TextModeratorAdapter,
	config *DuplicationDetectorConfig,
	next octollm.Engine,
) *DuplicationDetectorEngine {
	if config == nil {
		config = DefaultDuplicationDetectorConfig()
	}
	return &DuplicationDetectorEngine{
		next:    next,
		adapter: adapter,
		config:  config,
	}
}

// Process 处理请求
func (e *DuplicationDetectorEngine) Process(req *octollm.Request) (*octollm.Response, error) {
	resp, err := e.next.Process(req)
	if err != nil {
		return nil, err
	}

	// 提取模型信息用于日志
	modelName := e.extractModelName(req)

	// 非流式：同步提取文本，异步检测
	if resp.Body != nil {
		text, err := e.adapter.ExtractTextFromBody(req.Context(), resp.Body)
		if err != nil {
			logrus.WithContext(req.Context()).Warnf("extract text from response error: %v", err)
			return resp, nil // 不影响正常返回
		}

		// 异步检测
		go e.detectWithTimeout(req.Context(), text, modelName)
		return resp, nil
	}

	// 流式：收集完后异步检测
	return e.processStream(req, resp, modelName)
}

// processStream 处理流式响应
func (e *DuplicationDetectorEngine) processStream(req *octollm.Request, resp *octollm.Response, modelName string) (*octollm.Response, error) {
	inCh := resp.Stream.Chan()
	outCh := make(chan *octollm.StreamChunk)

	go func() {
		var textBuffer []rune

		// 转发所有 chunks 并收集文本
		for chunk := range inCh {
			outCh <- chunk

			// 收集文本
			text, err := e.adapter.ExtractTextFromBody(req.Context(), chunk.Body)
			if err == nil {
				textBuffer = append(textBuffer, text...)
			}
		}

		// 先关闭 outCh（让 HTTP 连接关闭）
		close(outCh)

		// 然后异步检测（不阻塞连接关闭）
		go e.detectWithTimeout(req.Context(), textBuffer, modelName)
	}()

	resp.Stream = octollm.NewStreamChan(outCh, resp.Stream.Close)
	return resp, nil
}

// detectWithTimeout 带超时的重复检测
func (e *DuplicationDetectorEngine) detectWithTimeout(ctx context.Context, text []rune, modelName string) {
	// 创建带超时的 context
	detectCtx, cancel := context.WithTimeout(context.Background(), e.config.DetectTimeout)
	defer cancel()

	startTime := time.Now()

	// 在新的 goroutine 中执行检测
	resultCh := make(chan struct {
		pattern     string
		repeatCount int
		found       bool
	}, 1)

	go func() {
		pattern, repeatCount, found := utils.ExtractRepeatPattern(
			string(text),
			e.config.MinRepeatLen,
			e.config.MaxRepeatLen,
			e.config.RepeatThreshold,
		)
		resultCh <- struct {
			pattern     string
			repeatCount int
			found       bool
		}{pattern, repeatCount, found}
	}()

	// 等待检测完成或超时
	select {
	case result := <-resultCh:
		detectionTime := time.Since(startTime)

		if result.found {
			e.logRepeatDetection(ctx, modelName, string(text), result.pattern, result.repeatCount, detectionTime)
		}
	case <-detectCtx.Done():
		logrus.WithContext(ctx).WithFields(logrus.Fields{
			"model":          modelName,
			"detection_time": time.Since(startTime),
			"content_length": len(text),
			"timeout":        e.config.DetectTimeout,
		}).Warn("[DuplicationDetector] Detection timeout")
	}
}

// logRepeatDetection 记录重复检测结果
func (e *DuplicationDetectorEngine) logRepeatDetection(ctx context.Context, modelName, content, pattern string, repeatCount int, detectionTime time.Duration) {
	// 截断 pattern 用于日志（避免太长）
	patternPreview := pattern
	if len(patternPreview) > 100 {
		patternPreview = patternPreview[:100] + "..."
	}

	traceID := extractTraceID(ctx)

	logrus.WithContext(ctx).WithFields(logrus.Fields{
		"model":          modelName,
		"trace_id":       traceID,
		"detection_time": detectionTime,
		"content_length": len([]rune(content)),
		"repeat_pattern": patternPreview,
		"repeat_count":   repeatCount,
		"pattern_length": len(pattern),
	}).Warn("[DuplicationDetector] Repeated pattern detected")
}

// extractModelName 从请求中提取模型名称
func (e *DuplicationDetectorEngine) extractModelName(req *octollm.Request) string {
	if req.Body == nil {
		return "unknown"
	}

	parsed, err := req.Body.Parsed()
	if err != nil {
		return "unknown"
	}

	switch body := parsed.(type) {
	case *openai.ChatCompletionRequest:
		return body.Model
	case *anthropic.ClaudeMessagesRequest:
		return body.Model
	default:
		return "unknown"
	}
}

// extractTraceID 从 context 中提取 trace_id
func extractTraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if traceID, ok := ctx.Value("trace_id").(string); ok {
		return traceID
	}
	return ""
}
