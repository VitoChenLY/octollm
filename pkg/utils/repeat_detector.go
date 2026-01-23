package utils

import (
	"strings"
)

// ExtractRepeatPattern 提取重复的 pattern 和重复次数
// 返回：重复的pattern内容，重复次数，是否找到重复
func ExtractRepeatPattern(text string, minRepeatLen, maxRepeatLen, repeatThreshold int) (pattern string, repeatCount int, found bool) {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	n := len(runes)

	if n < minRepeatLen*repeatThreshold {
		return "", 0, false
	}

	return extractRepeatPattern(runes, minRepeatLen, maxRepeatLen, repeatThreshold)
}

// extractRepeatPattern 使用后缀比较的重复检测算法
func extractRepeatPattern(runes []rune, minRepeatLen, maxRepeatLen, repeatThreshold int) (pattern string, repeatCount int, found bool) {
	n := len(runes)
	if n < minRepeatLen*repeatThreshold {
		return "", 0, false
	}

	// 计算 pattern 长度的上限
	maxPatternLen := n / repeatThreshold
	if maxRepeatLen > 0 && maxRepeatLen < maxPatternLen {
		maxPatternLen = maxRepeatLen
	}

	// 检查不同长度的 pattern（从小到大）
	for patternLen := minRepeatLen; patternLen <= maxPatternLen; patternLen++ {
		total := repeatThreshold * patternLen
		if total > n {
			continue
		}

		// 取末尾 repeatThreshold * patternLen 长度的片段
		tail := runes[n-total:]

		// 核心判定：如果 tail 去掉第一个 pattern 后，等于 tail 去掉最后一个 pattern
		// 则说明整个 tail 由相同的 pattern 重复了 repeatThreshold 次
		if runeSliceEqual(tail[patternLen:], tail[:len(tail)-patternLen]) {
			patternRunes := runes[n-patternLen:]
			return string(patternRunes), repeatThreshold, true
		}
	}

	return "", 0, false
}

// runeSliceEqual 精确比较两个 rune 切片是否相等
func runeSliceEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
