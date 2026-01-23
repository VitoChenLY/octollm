package moderator

import (
	"context"
	"fmt"
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
	// 是否拦截重复内容（默认 false，只记录日志不拦截）
	BlockOnDetect bool
	// 拦截时返回的消息（默认为通用提示）
	BlockMessage string
}

// DefaultDuplicationDetectorConfig 返回默认配置
func DefaultDuplicationDetectorConfig() *DuplicationDetectorConfig {
	return &DuplicationDetectorConfig{
		MinRepeatLen:    1,                                                          // 最小 1 个字符
		MaxRepeatLen:    5,                                                          // 最大 5 个字符
		RepeatThreshold: 50,                                                         // 重复 50 次以上
		BlockOnDetect:   false,                                                      // 默认不拦截
		BlockMessage:    "Repeated content detected, please adjust and try again. ", // 默认拦截消息
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

func (s *DuplicationDetectorService) Allow(ctx context.Context, text []rune) error {
	startTime := time.Now()

	pattern, repeatCount, found := utils.ExtractRepeatPattern(
		string(text),
		s.config.MinRepeatLen,
		s.config.MaxRepeatLen,
		s.config.RepeatThreshold,
	)

	detectionTime := time.Since(startTime)

	if found {
		s.logRepeatDetection(ctx, string(text), pattern, repeatCount, detectionTime)

		if s.config.BlockOnDetect {
			return fmt.Errorf("Content filter: pattern '%s' repeated %d times", pattern, repeatCount)
		}
	}

	return nil
}

// MaxRuneLen 返回最大检测文本长度
func (s *DuplicationDetectorService) MaxRuneLen() int {
	return 2000
}

// logRepeatDetection 记录重复检测结果
func (s *DuplicationDetectorService) logRepeatDetection(ctx context.Context, content, pattern string, repeatCount int, detectionTime time.Duration) {
	// 截断 pattern 用于日志（避免太长）
	patternPreview := pattern
	if len(patternPreview) > 100 {
		patternPreview = patternPreview[:100] + "..."
	}

	traceID := extractTraceID(ctx)
	svcName := extractBackendName(ctx)

	logrus.WithContext(ctx).WithFields(logrus.Fields{
		"model":          s.modelName,
		"svc_name":       svcName,
		"trace_id":       traceID,
		"detection_time": detectionTime,
		"content_length": len([]rune(content)),
		"repeat_pattern": patternPreview,
		"repeat_count":   repeatCount,
		"pattern_length": len(pattern),
	}).Warn("[DuplicationDetector] Repeated pattern detected")

	// Record metrics to Grafana
	RepeatDetectionCounter.WithLabelValues(svcName, s.modelName).Inc()
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

// extractBackendName 从 context 中提取 backend_name
func extractBackendName(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if backendName, ok := ctx.Value("backend_name").(string); ok {
		return backendName
	}
	return ""
}
