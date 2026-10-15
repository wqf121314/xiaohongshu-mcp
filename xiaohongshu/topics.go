package xiaohongshu

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// TopicResult 是一个话题在发布时的关联结果。
type TopicResult struct {
	Name   string `json:"name"`
	Linked bool   `json:"linked"` // 是否在联想列表里选中了同名话题
}

// TopicInfo 是搜索到的话题及其浏览量。
type TopicInfo struct {
	Name      string `json:"name"`
	Views     int64  `json:"views"`
	ViewsText string `json:"views_text"`
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

// FormatTopicList 把搜索到的话题格式化成每行一个，最多 max 行（max<=0 不限制）。
func FormatTopicList(ts []TopicInfo, max int) string {
	if len(ts) == 0 {
		return "没有找到相关话题"
	}
	if max > 0 && len(ts) > max {
		ts = ts[:max]
	}
	lines := make([]string, 0, len(ts))
	for _, t := range ts {
		if t.ViewsText == "" {
			lines = append(lines, "#"+t.Name)
		} else {
			lines = append(lines, "#"+t.Name+"（"+t.ViewsText+"）")
		}
	}
	return strings.Join(lines, "\n")
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

// maxTags 每篇笔记最多的话题数
const maxTags = 10

// checkTagsLimit 话题超过上限时报错，不再悄悄截断。
func checkTagsLimit(tags []string) error {
	if len(tags) > maxTags {
		return errors.New("话题最多 10 个")
	}
	return nil
}

// topicSearchResp 是创作平台话题搜索接口（/search/topics）的返回结构。
type topicSearchResp struct {
	Success bool   `json:"success"`
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    struct {
		Topics []struct {
			Info struct {
				Name           string `json:"name"`
				ViewNum        int64  `json:"view_num"`
				DiscussNumDesc string `json:"discuss_num_desc"`
			} `json:"topic_info_dto"`
		} `json:"topics"`
	} `json:"data"`
}

// ParseTopicSearchResponse 解析话题搜索接口的返回，顺序与联想列表一致。
func ParseTopicSearchResponse(body []byte) ([]TopicInfo, error) {
	var r topicSearchResp
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("解析话题搜索结果失败: %w", err)
	}
	if !r.Success || r.Code != 0 {
		return nil, fmt.Errorf("话题搜索失败: code=%d msg=%s", r.Code, r.Msg)
	}
	topics := make([]TopicInfo, 0, len(r.Data.Topics))
	for _, t := range r.Data.Topics {
		topics = append(topics, TopicInfo{
			Name:      t.Info.Name,
			Views:     t.Info.ViewNum,
			ViewsText: t.Info.DiscussNumDesc,
		})
	}
	return topics, nil
}

// topicSearchKeyword 从话题搜索请求体中取出关键词；ok 为 false 表示读不出关键词。
func topicSearchKeyword(postData string) (keyword string, ok bool) {
	var req struct {
		Keyword *string `json:"keyword"`
	}
	if err := json.Unmarshal([]byte(postData), &req); err != nil || req.Keyword == nil {
		return "", false
	}
	return *req.Keyword, true
}

// MatchDropdownItem 核对联想列表某项的文字是否就是该话题：
// 去掉浏览量文字、所有空白和开头的 # 后，必须与话题名完全相同。
func MatchDropdownItem(itemText, name, viewsText string) bool {
	if viewsText != "" {
		// 浏览量在名字之后，去掉最后一次出现
		if i := strings.LastIndex(itemText, viewsText); i >= 0 {
			itemText = itemText[:i] + itemText[i+len(viewsText):]
		}
	}
	want := strings.TrimLeft(removeSpaces(name), "#")
	return want != "" && strings.TrimLeft(removeSpaces(itemText), "#") == want
}

// removeSpaces 去掉所有空白字符（含全角空格、换行）。
func removeSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}
