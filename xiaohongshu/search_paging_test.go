package xiaohongshu

import (
	"slices"
	"testing"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/configs"
)

func TestPublishTimeFromID(t *testing.T) {
	cases := map[string]int64{
		"6ad433a6000000000c00c7c9": 1792291750000, // 0x6ad433a6 秒 × 1000
		"6AD433A6000000000C00C7C9": 0,             // 只认小写，和站点一致
		"abc":                      0,
		"":                         0,
		"zzzzzzzz000000000c00c7c9": 0,
	}
	for id, want := range cases {
		if got := publishTimeFromID(id); got != want {
			t.Errorf("publishTimeFromID(%q) = %d, want %d", id, got, want)
		}
	}
}

func TestPageOptionsDefaults(t *testing.T) {
	opts, err := ParsePageOptions(0, "", 0)
	if err != nil || opts.MaxPages != 1 || !opts.Since.IsZero() {
		t.Fatalf("got %+v, %v", opts, err)
	}
	if opts, _ := ParsePageOptions(9, "", 0); opts.MaxPages != configs.DefaultMaxSearchPages {
		t.Fatalf("max pages not capped: %d", opts.MaxPages)
	}
	if opts, _ := ParsePageOptions(-2, "", 0); opts.MaxPages != 1 {
		t.Fatalf("negative max pages: %d", opts.MaxPages)
	}
}

func TestParsePageOptionsSince(t *testing.T) {
	opts, err := ParsePageOptions(3, "2026-10-13T00:00:00+13:00", 0)
	if err != nil || opts.MaxPages != 3 || opts.Since.Unix() != 1791802800 {
		t.Fatalf("got %+v, %v", opts, err)
	}
}

func TestParsePageOptionsRejectsBadSince(t *testing.T) {
	if _, err := ParsePageOptions(3, "2026-10-13", 0); err == nil {
		t.Fatal("want error for non-RFC3339 since")
	}
}

func TestParsePageOptionsMaxSeconds(t *testing.T) {
	opts, err := ParsePageOptions(3, "", 50)
	if err != nil || opts.MaxDuration != 50*time.Second {
		t.Fatalf("got %+v, %v", opts, err)
	}
	if opts, _ := ParsePageOptions(3, "", 0); opts.MaxDuration != 0 {
		t.Fatalf("0 应该表示不限，got %v", opts.MaxDuration)
	}
	if _, err := ParsePageOptions(3, "", -1); err == nil {
		t.Fatal("want error for negative max_seconds")
	}
}

func TestOutOfTime(t *testing.T) {
	now := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	check := func(name string, deadline time.Time, slowest time.Duration, want bool) {
		t.Helper()
		if got := outOfTime(deadline, now, slowest); got != want {
			t.Errorf("%s: got %v want %v", name, got, want)
		}
	}
	check("no deadline", time.Time{}, time.Hour, false)
	check("plenty of time", now.Add(30*time.Second), 6*time.Second, false)
	// 下一页按最慢的一页估计，再留出关浏览器、返回结果的时间
	check("slowest page would overrun", now.Add(12*time.Second), 11*time.Second, true)
	// 还没翻过页时按 pageTurnEstimate 估计
	check("first turn uses estimate", now.Add(pageTurnEstimate+closeReserve-time.Second), 0, true)
	check("first turn fits", now.Add(pageTurnEstimate+closeReserve+time.Second), 0, false)
	check("deadline passed", now.Add(-time.Second), 0, true)
}

func TestShouldStop(t *testing.T) {
	since := time.Date(2026, 10, 13, 0, 0, 0, 0, time.FixedZone("UTC+13", 13*3600))
	opts := PageOptions{MaxPages: 3, Since: since}
	newer := Feed{PublishTime: since.Add(time.Hour).UnixMilli()}
	older := Feed{PublishTime: since.Add(-time.Hour).UnixMilli()}
	unknown := Feed{PublishTime: 0}
	n := func(f Feed, count int) []Feed { return slices.Repeat([]Feed{f}, count) }
	join := func(parts ...[]Feed) []Feed { return slices.Concat(parts...) }

	// feeds 是目前拿到的全部结果，added 是最后一页新增的条数
	check := func(name string, opts PageOptions, page int, feeds []Feed, added int, wantStop, wantReached bool) {
		t.Helper()
		stop, reached := shouldStop(opts, page, feeds, added)
		if stop != wantStop || reached != wantReached {
			t.Errorf("%s: got (%v,%v) want (%v,%v)", name, stop, reached, wantStop, wantReached)
		}
	}
	check("all newer, more pages", opts, 1, n(newer, 20), 20, false, false)
	// 「最新」排序夹着零星旧帖：末尾 5 条里 2 条旧帖，中位数仍晚于 since
	check("scattered older keeps paging", opts, 2,
		join(n(newer, 20), n(newer, 15), []Feed{older, newer, newer, older, newer}), 20, false, false)
	check("whole page older stops", opts, 2, join(n(newer, 20), n(older, 20)), 20, true, true)
	check("first page all older stops", opts, 1, n(older, 20), 20, true, true)
	check("3 of last 5 older stops", opts, 2,
		join(n(newer, 35), []Feed{older, newer, older, newer, older}), 20, true, true)
	// 偶数条取较晚的那个，和"上中位数"一致
	check("even tail takes later middle", opts, 1, []Feed{older, older, newer, newer}, 4, false, false)
	check("unknown time skipped in tail", opts, 2, join(n(older, 20), n(unknown, 5)), 5, true, true)
	check("all unknown keeps paging", opts, 1, n(unknown, 20), 20, false, false)
	check("max pages", opts, 3, n(newer, 60), 20, true, false)
	check("max pages and older", opts, 3, join(n(newer, 40), n(older, 20)), 20, true, true)
	check("no new feeds", opts, 2, n(newer, 20), 0, true, false)
	check("no since", PageOptions{MaxPages: 3}, 1, n(older, 20), 20, false, false)
	check("no since, max pages", PageOptions{MaxPages: 3}, 3, n(older, 60), 20, true, false)
	check("single page", PageOptions{MaxPages: 1}, 1, n(newer, 20), 20, true, false)
}

func TestDetectBlocked(t *testing.T) {
	if detectBlocked("请完成安全验证后继续") != "安全验证" {
		t.Fatal("miss")
	}
	if detectBlocked("操作频繁，请稍后再试") != "操作频繁" {
		t.Fatal("miss")
	}
	if detectBlocked("闲置 搜索结果") != "" {
		t.Fatal("false positive")
	}
}

func TestParsePageOptionsUsesConfiguredLimit(t *testing.T) {
	t.Cleanup(func() { _ = configs.SetMaxSearchPages(configs.DefaultMaxSearchPages) })
	if err := configs.SetMaxSearchPages(10); err != nil {
		t.Fatal(err)
	}
	if opts, _ := ParsePageOptions(0, "", 0); opts.MaxPages != 1 {
		t.Fatalf("default pages changed: %d", opts.MaxPages)
	}
	if opts, _ := ParsePageOptions(11, "", 0); opts.MaxPages != 10 {
		t.Fatalf("want 10, got %d", opts.MaxPages)
	}
	if opts, _ := ParsePageOptions(10, "", 0); opts.MaxPages != 10 {
		t.Fatalf("want 10, got %d", opts.MaxPages)
	}
}
