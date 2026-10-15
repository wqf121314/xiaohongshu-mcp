package xiaohongshu

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// TopicResult 是一个话题在发布时的关联结果。
type TopicResult struct {
	Name   string `json:"name"`
	Linked bool   `json:"linked"` // 是否在联想列表里选中了同名话题
}

// TopicInfo 是搜索到的话题及其浏览量。
type TopicInfo struct {
	Name  string `json:"name"`
	Views int64  `json:"views"`
}

const topicSuffix = "[话题]"

// NormalizeTopic 规范化话题名：去首尾空白、开头的 #、结尾的 [话题] 和 #。
func NormalizeTopic(s string) string {
	for {
		prev := s
		s = strings.TrimSpace(s)
		s = strings.TrimLeft(s, "#")
		s = strings.TrimRight(s, "#")
		s = strings.TrimSuffix(s, topicSuffix)
		s = strings.TrimSpace(s)
		if s == prev {
			return s
		}
	}
}

// PickTopic 返回与 want 规范化后完全相同的第一项下标，没有则返回 -1。
func PickTopic(want string, options []string) int {
	w := NormalizeTopic(want)
	if w == "" {
		return -1
	}
	for i, opt := range options {
		if NormalizeTopic(opt) == w {
			return i
		}
	}
	return -1
}

// FormatTopicResults 把话题关联结果格式化成一句提示，没有结果返回空串。
func FormatTopicResults(rs []TopicResult) string {
	if len(rs) == 0 {
		return ""
	}
	items := make([]string, 0, len(rs))
	for _, r := range rs {
		if r.Linked {
			items = append(items, "#"+r.Name+" ✅已关联")
		} else {
			items = append(items, "#"+r.Name+" ⚠️未找到同名话题，按普通文字发出")
		}
	}
	return "话题：" + strings.Join(items, "；")
}

var viewCountRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(亿|万|w|W)?\s*(?:次\s*浏览|浏览|次)?$`)

// ParseViewCount 解析浏览量文本，如 "1.2亿次浏览"、"3.4万"、"856次浏览"，认不出返回 0。
func ParseViewCount(s string) int64 {
	m := viewCountRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0
	}
	num, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	switch m[2] {
	case "亿":
		num *= 1e8
	case "万", "w", "W":
		num *= 1e4
	}
	// 超出 int64 范围视为认不出
	if num >= math.MaxInt64 {
		return 0
	}
	return int64(math.Round(num))
}
