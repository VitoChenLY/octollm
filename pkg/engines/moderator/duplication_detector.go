package moderator

import (
	"context"
	"time"

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

// DuplicationDetectorService 实现 TextModeratorService 接口
// 用于集成到 TextModeratorEngine 中，复用流式处理逻辑
type DuplicationDetectorService struct {
	config    *DuplicationDetectorConfig
	modelName string
}

var _ TextModeratorService = (*DuplicationDetectorService)(nil)

// NewDuplicationDetectorService 创建重复检测服务
func NewDuplicationDetectorService(
	config *DuplicationDetectorConfig,
	modelName string,
) *DuplicationDetectorService {
	if config == nil {
		config = DefaultDuplicationDetectorConfig()
	}
	return &DuplicationDetectorService{
		config:    config,
		modelName: modelName,
	}
}

// Allow 检测文本重复，异步执行不阻塞响应
func (s *DuplicationDetectorService) Allow(ctx context.Context, text []rune) error {
	// 异步检测，不阻塞响应
	go s.detectWithTimeout(ctx, text)
	return nil // 永远返回 nil，不拦截响应
}

// MaxRuneLen 返回最大检测文本长度
func (s *DuplicationDetectorService) MaxRuneLen() int {
	return 2000
}

// detectWithTimeout 带超时的异步检测
func (s *DuplicationDetectorService) detectWithTimeout(ctx context.Context, text []rune) {
	// 创建带超时的 context
	detectCtx, cancel := context.WithTimeout(context.Background(), s.config.DetectTimeout)
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
			s.config.MinRepeatLen,
			s.config.MaxRepeatLen,
			s.config.RepeatThreshold,
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
			s.logRepeatDetection(ctx, string(text), result.pattern, result.repeatCount, detectionTime)
		}
	case <-detectCtx.Done():
		logrus.WithContext(ctx).WithFields(logrus.Fields{
			"model":          s.modelName,
			"detection_time": time.Since(startTime),
			"content_length": len(text),
			"timeout":        s.config.DetectTimeout,
		}).Warn("[DuplicationDetector] Detection timeout")
	}
}

// logRepeatDetection 记录重复检测结果
func (s *DuplicationDetectorService) logRepeatDetection(ctx context.Context, content, pattern string, repeatCount int, detectionTime time.Duration) {
	// 截断 pattern 用于日志（避免太长）
	patternPreview := pattern
	if len(patternPreview) > 100 {
		patternPreview = patternPreview[:100] + "..."
	}

	traceID := extractTraceID(ctx)

	logrus.WithContext(ctx).WithFields(logrus.Fields{
		"model":          s.modelName,
		"trace_id":       traceID,
		"detection_time": detectionTime,
		"content_length": len([]rune(content)),
		"repeat_pattern": patternPreview,
		"repeat_count":   repeatCount,
		"pattern_length": len(pattern),
	}).Warn("[DuplicationDetector] Repeated pattern detected")
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
