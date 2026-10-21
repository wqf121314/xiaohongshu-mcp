package xiaohongshu

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxSearchPages 单次搜索最多翻几页（第一屏算 1 页）。
const MaxSearchPages = 5

// PageOptions 搜索翻页选项。
type PageOptions struct {
	MaxPages int       // 1..MaxSearchPages
	Since    time.Time // 翻到早于这个时间的帖子就停；零值表示不限
}

// ParsePageOptions 校验并规整翻页参数：maxPages 不填按 1，超过上限按上限；since 为 RFC 3339。
// 在打开浏览器之前调用，写错的参数不该先访问平台再报错。
func ParsePageOptions(maxPages int, since string) (PageOptions, error) {
	opts := PageOptions{MaxPages: maxPages}
	if opts.MaxPages < 1 {
		opts.MaxPages = 1
	}
	if opts.MaxPages > MaxSearchPages {
		opts.MaxPages = MaxSearchPages
	}
	if since = strings.TrimSpace(since); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return opts, fmt.Errorf("since 需要 RFC 3339 格式（如 2026-01-02T00:00:00+08:00）: %q", since)
		}
		opts.Since = t
	}
	return opts, nil
}

// shouldStop 判断读完第 page 页后是否停止翻页；newFeeds 是这一页新增的帖子。
// 返回的 reachedSince 表示这一页已出现早于 Since 的帖子。
func shouldStop(opts PageOptions, page int, newFeeds []Feed) (stop bool, reachedSince bool) {
	if !opts.Since.IsZero() {
		cutoff := opts.Since.UnixMilli()
		for _, f := range newFeeds {
			// 时间未知的不参与判断
			if f.PublishTime > 0 && f.PublishTime < cutoff {
				return true, true
			}
		}
	}
	if page >= opts.MaxPages || len(newFeeds) == 0 {
		return true, false
	}
	return false, false
}

// blockedMarks 风控页面上的字样。
var blockedMarks = []string{"验证码", "滑块", "操作频繁", "安全验证"}

// detectBlocked 页面文字命中风控字样时返回该字样，否则返回空。
func detectBlocked(bodyText string) string {
	for _, mark := range blockedMarks {
		if strings.Contains(bodyText, mark) {
			return mark
		}
	}
	return ""
}

// noteIDPattern 笔记 id：24 位小写十六进制，前 8 位是创建时间（Unix 秒）。
var noteIDPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// publishTimeFromID 由笔记 id 换算发布时间（Unix 毫秒），id 格式不对时返回 0。
func publishTimeFromID(id string) int64 {
	if !noteIDPattern.MatchString(id) {
		return 0
	}
	secs, err := strconv.ParseInt(id[:8], 16, 64)
	if err != nil {
		return 0
	}
	return secs * 1000
}

// withPublishTime 为每条结果填上由 id 换算的发布时间。
func withPublishTime(feeds []Feed) []Feed {
	for i := range feeds {
		feeds[i].PublishTime = publishTimeFromID(feeds[i].ID)
	}
	return feeds
}
