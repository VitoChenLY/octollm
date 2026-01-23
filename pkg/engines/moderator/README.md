# Moderator Adapters

本目录包含用于内容审核的适配器实现，支持多种 AI API 格式。

## 架构概览

```
TextModeratorAdapter (接口)
    ├── OpenAIAdapter        - OpenAI Chat Completions API
    ├── ClaudeAdapter        - Anthropic Messages API
    └── UniversalAdapter     - 通用适配器（自动识别格式）
```

## 核心组件

### 1. TextModeratorAdapter 接口

所有适配器都实现此接口：

```go
type TextModeratorAdapter interface {
    // 从请求/响应中提取文本内容
    ExtractTextFromBody(ctx context.Context, body *octollm.UnifiedBody) ([]rune, error)
    
    // 生成替换的响应体（用于内容拦截）
    GetReplacementBody(ctx context.Context, body *octollm.UnifiedBody) *octollm.UnifiedBody
}
```

### 2. OpenAIAdapter

支持 OpenAI Chat Completions API 格式：
- `ChatCompletionRequest`
- `ChatCompletionResponse`
- `ChatCompletionStreamChunk`

**功能：**
- 提取消息内容、工具调用参数、推理内容
- 支持流式和非流式响应
- 生成符合 OpenAI 格式的替换响应

### 3. ClaudeAdapter

支持 Anthropic Messages API 格式：
- `ClaudeMessagesRequest`
- `ClaudeMessagesResponse`
- `ClaudeMessagesStreamEvent`

**功能：**
- 提取系统提示、消息内容、工具使用、思考内容
- 支持流式 delta 事件
- 生成符合 Claude 格式的替换响应

### 4. UniversalAdapter（推荐使用）

**自动识别 API 格式并调用相应的适配器**，支持：
- OpenAI Chat Completions 格式
- Anthropic Messages 格式

**使用示例：**

```go
// 创建通用适配器
adapter := moderator.NewUniversalAdapter()

// 或带配置创建
adapter := moderator.NewUniversalAdapterWithConfig(
    "替换流式文本",
    "替换非流式文本",
    "stop",        // OpenAI finish_reason
    "end_turn",    // Claude stop_reason
)

// 自动识别格式并提取文本
text, err := adapter.ExtractTextFromBody(ctx, body)

// 自动识别格式并生成替换响应
replacementBody := adapter.GetReplacementBody(ctx, body)
```

## DuplicationDetectorService

重复内容检测服务，使用 `UniversalAdapter` 实现跨格式支持。

**配置：**

```yaml
duplication_detection:
  enabled: true
  min_repeat_len: 1            # 最小重复长度
  max_repeat_len: 5            # 最大重复长度
  repeat_threshold: 50         # 重复次数阈值
  block_on_detect: false       # 是否拦截（默认只记录日志）
  block_message: "检测到内容存在异常重复，请调整后重试。若有疑问，请联系技术支持。"  # 拦截时返回的消息
  moderate_stream_every: 50    # 流式检测频率
```

**功能特性：**
- ✅ 同步检测（高性能，3-9μs）
- ✅ 支持 OpenAI 和 Claude 格式
- ✅ 可配置拦截或仅记录
- ✅ 拦截时返回友好的替换消息（类似内容审核）
- ✅ 流式和非流式响应均支持
- ✅ Unicode 字符完整支持

## 拦截消息机制

当 `block_on_detect: true` 时，重复检测器会拦截重复内容并返回替换消息，而不是返回错误。

**工作流程：**

1. **检测到重复** → 记录日志 + 触发拦截
2. **生成替换响应** → 使用 `block_message` 作为内容
3. **返回正常响应** → HTTP 200，但内容被替换

**OpenAI 格式的拦截响应：**
```json
{
  "id": "original-id",
  "model": "gpt-4",
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "检测到内容存在异常重复，请调整后重试。若有疑问，请联系技术支持。"
    },
    "finish_reason": "content_filter"
  }]
}
```

**Claude 格式的拦截响应：**
```json
{
  "id": "original-id",
  "type": "message",
  "role": "assistant",
  "content": [{
    "type": "text",
    "text": "检测到内容存在异常重复，请调整后重试。若有疑问，请联系技术支持。"
  }],
  "model": "claude-3-5-sonnet-20241022",
  "stop_reason": "content_filter"
}
```

**流式响应的拦截：**
```
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"检测到内容存在异常重复，请调整后重试。若有疑问，请联系技术支持。"}}

data: {"type":"message_delta","delta":{"stop_reason":"content_filter"}}

data: [DONE]
```

## 使用场景

### 场景 1：重复内容检测

```go
// 自动支持 OpenAI 和 Claude
detector := &moderator.TextModeratorEngine{
    ModeratorService:     service,
    TextModeratorAdapter: moderator.NewUniversalAdapter(),
    ModerateOutput:       true,
    ModerateStreamEvery:  50,
    Next:                 nextEngine,
}
```

### 场景 2：自定义内容审核

```go
// 实现自定义审核逻辑
type MyModeratorService struct {
    config *MyConfig
}

func (s *MyModeratorService) Allow(ctx context.Context, text []rune) error {
    // 自定义审核逻辑
    if containsBadWords(text) {
        return fmt.Errorf("内容包含敏感词")
    }
    return nil
}

// 使用通用 adapter
engine := &moderator.TextModeratorEngine{
    ModeratorService:     myService,
    TextModeratorAdapter: moderator.NewUniversalAdapter(),
    ModerateInput:        true,
    ModerateOutput:       true,
}
```

## 测试

```bash
# 测试所有适配器
go test -v ./pkg/engines/moderator/

# 只测试 Claude adapter
go test -v ./pkg/engines/moderator/ -run TestClaudeAdapter

# 只测试 Universal adapter
go test -v ./pkg/engines/moderator/ -run TestUniversalAdapter

# 测试重复检测
go test -v ./pkg/engines/moderator/ -run TestDuplicationDetector
```

## 性能指标

基于实际测试数据：

| 操作 | 响应时间 | 说明 |
|------|---------|------|
| 重复检测（短文本） | ~4-7μs | 45 字符，重复 3 次 |
| 重复检测（中等文本） | ~4-9μs | 100-120 字符，重复 50 次 |
| 重复检测（长文本） | ~4μs | 180 字符，重复 50 次 |

**结论：** 新的后缀比较算法性能优异，适合生产环境使用。

## 扩展指南

要添加新的 API 格式支持：

1. 创建新的 adapter 文件（如 `gemini_adapter.go`）
2. 实现 `TextModeratorAdapter` 接口
3. 在 `UniversalAdapter` 中添加类型检测和路由逻辑
4. 编写测试用例

```go
// 示例：添加 Gemini 支持
type GeminiAdapter struct {
    // ... 配置
}

func (a *GeminiAdapter) ExtractTextFromBody(ctx context.Context, body *octollm.UnifiedBody) ([]rune, error) {
    // 实现 Gemini 格式解析
}

func (a *GeminiAdapter) GetReplacementBody(ctx context.Context, body *octollm.UnifiedBody) *octollm.UnifiedBody {
    // 实现 Gemini 格式替换
}

// 在 UniversalAdapter 中添加
func (a *UniversalAdapter) ExtractTextFromBody(ctx context.Context, body *octollm.UnifiedBody) ([]rune, error) {
    parsed, _ := body.Parsed()
    switch parsed.(type) {
    case *openai.ChatCompletionRequest, ...:
        return a.openaiAdapter.ExtractTextFromBody(ctx, body)
    case *anthropic.ClaudeMessagesRequest, ...:
        return a.claudeAdapter.ExtractTextFromBody(ctx, body)
    case *gemini.GenerateContentRequest, ...:  // 新增
        return a.geminiAdapter.ExtractTextFromBody(ctx, body)
    }
}
```

## 注意事项

1. **优先使用 UniversalAdapter**：除非有特殊需求，否则建议使用通用适配器
2. **Unicode 支持**：所有 adapter 返回 `[]rune` 类型，正确处理多字节字符
3. **错误处理**：解析失败时返回错误，不会 panic
4. **性能优化**：重复检测使用同步模式，避免 goroutine 开销
5. **配置验证**：使用默认配置时会自动填充合理的默认值

## 相关文档

- [重复检测算法说明](../../utils/repeat_detector.go)
- [配置文件格式](../../../examples/config-repeat-check.yaml)
- [TextModeratorEngine 文档](./moderator.go)
