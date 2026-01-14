package utils

import (
	"strings"
)

const (
	// 滚动哈希参数
	hashBase = 31      // 质数基数
	hashMod  = 1e9 + 7 // 大质数模数
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

	return extractRepeatPatternWithHash(runes, minRepeatLen, maxRepeatLen, repeatThreshold)
}

// extractRepeatPatternWithHash 使用滚动哈希优化的重复检测算法
func extractRepeatPatternWithHash(runes []rune, minRepeatLen, maxRepeatLen, repeatThreshold int) (pattern string, repeatCount int, found bool) {
	n := len(runes)
	if n < minRepeatLen*repeatThreshold {
		return "", 0, false
	}

	// 计算 pattern 长度的上限
	maxPatternLen := n / repeatThreshold
	if maxRepeatLen > 0 && maxRepeatLen < maxPatternLen {
		maxPatternLen = maxRepeatLen
	}

	// 检查不同长度的 pattern（从小到大，更容易早期找到）
	for patternLen := minRepeatLen; patternLen <= maxPatternLen; patternLen++ {
		// 从文本末尾提取 pattern
		patternRunes := runes[n-patternLen:]

		// 计算末尾 pattern 的哈希值
		patternHash := computeHash(patternRunes)

		// 预计算 base^patternLen，用于滚动哈希
		basePower := int64(1)
		for i := 0; i < patternLen; i++ {
			basePower = (basePower * hashBase) % hashMod
		}

		consecutiveMatches := 1

		// 向前检查是否有连续的重复
		for i := n - patternLen*2; i >= 0 && consecutiveMatches < repeatThreshold; i -= patternLen {
			// 计算当前位置的哈希值
			currentHash := computeHash(runes[i : i+patternLen])

			// 哈希匹配时，再逐字符确认（避免哈希冲突）
			if currentHash == patternHash && exactMatch(runes[i:i+patternLen], patternRunes) {
				consecutiveMatches++
				if consecutiveMatches >= repeatThreshold {
					return string(patternRunes), consecutiveMatches, true
				}
			} else {
				// 不匹配，跳出（只检测连续重复）
				break
			}
		}
	}

	return "", 0, false
}

// computeHash 计算字符串的滚动哈希值
// hash = (c[0] * base^(n-1) + c[1] * base^(n-2) + ... + c[n-1]) % mod
func computeHash(runes []rune) int64 {
	hash := int64(0)
	for _, r := range runes {
		hash = (hash*hashBase + int64(r)) % hashMod
	}
	return hash
}

// exactMatch 精确比较两个 rune 切片是否相等
func exactMatch(a, b []rune) bool {
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
