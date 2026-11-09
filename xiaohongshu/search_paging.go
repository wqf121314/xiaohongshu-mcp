package xiaohongshu

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/configs"
)

// PageOptions 搜索翻页选项。
type PageOptions struct {
	MaxPages    int           // 1..configs.MaxSearchPages()
	Since       time.Time     // 结果末尾早于这个时间就停，见 tailTime；零值表示不限
	MaxDuration time.Duration // 整次搜索最多用多久，0 表示不限；由调用方换算成 Deadline
	Deadline    time.Time     // 快到这个时间就不再翻页，见 outOfTime；零值表示不限
}

// 判断还来不及翻下一页时用：还没翻过页时，下一页按 pageTurnEstimate 估计（翻页前停 3–6 秒，再加载）；
// closeReserve 留给关浏览器、返回结果。
const (
	pageTurnEstimate = 8 * time.Second
	closeReserve     = 2 * time.Second
)

// ParsePageOptions 校验并规整翻页参数：maxPages 不填按 1，超过上限按上限；since 为 RFC 3339；
// maxSeconds 为 0 表示不限。在打开浏览器之前调用，写错的参数不该先访问平台再报错。
func ParsePageOptions(maxPages int, since string, maxSeconds int) (PageOptions, error) {
	if maxSeconds < 0 {
		return PageOptions{}, fmt.Errorf("max_seconds 不能为负数: %d", maxSeconds)
	}
	opts := PageOptions{MaxPages: maxPages, MaxDuration: time.Duration(maxSeconds) * time.Second}
	if opts.MaxPages < 1 {
		opts.MaxPages = 1
	}
	if limit := configs.MaxSearchPages(); opts.MaxPages > limit {
		opts.MaxPages = limit
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

// outOfTime 判断按目前最慢的一页估计，翻下一页会不会超过 deadline；deadline 为零值时不限。
func outOfTime(deadline, now time.Time, slowest time.Duration) bool {
	if deadline.IsZero() {
		return false
	}
	return now.Add(max(slowest, pageTurnEstimate) + closeReserve).After(deadline)
}

// sinceTail 判断是否翻到 Since 时看结果末尾几条。
const sinceTail = 5

// shouldStop 判断读完第 page 页后是否停止翻页；feeds 是目前拿到的全部结果，added 是这一页新增的条数。
// 返回的 reachedSince 表示结果末尾已早于 Since，见 tailTime。
func shouldStop(opts PageOptions, page int, feeds []Feed, added int) (stop bool, reachedSince bool) {
	if !opts.Since.IsZero() {
		if t := tailTime(feeds); t > 0 && t < opts.Since.UnixMilli() {
			return true, true
		}
	}
	if page >= opts.MaxPages || added == 0 {
		return true, false
	}
	return false, false
}

// tailTime 结果最后 sinceTail 条发布时间的中位数，偶数条取较晚的；时间未知的跳过，都未知时返回 0。
//
// 「最新」排序并不严格按时间，常夹着几条旧帖。只要出现一条早于 Since 就停的话，
// 往往第 2 页就停了，max_pages 形同虚设。看末尾中位数，一两条旧帖不会让翻页提前结束，
// 整页都是旧帖时照样停下。取上中位数是为了少停：宁可多翻一页，也不提前停。
func tailTime(feeds []Feed) int64 {
	times := make([]int64, 0, sinceTail)
	for i := len(feeds) - 1; i >= 0 && len(times) < sinceTail; i-- {
		if t := feeds[i].PublishTime; t > 0 {
			times = append(times, t)
		}
	}
	if len(times) == 0 {
		return 0
	}
	slices.Sort(times)
	return times[len(times)/2]
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
