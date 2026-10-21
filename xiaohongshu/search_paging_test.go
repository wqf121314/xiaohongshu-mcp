package xiaohongshu

import (
	"testing"
	"time"
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
	opts, err := ParsePageOptions(0, "")
	if err != nil || opts.MaxPages != 1 || !opts.Since.IsZero() {
		t.Fatalf("got %+v, %v", opts, err)
	}
	if opts, _ := ParsePageOptions(9, ""); opts.MaxPages != MaxSearchPages {
		t.Fatalf("max pages not capped: %d", opts.MaxPages)
	}
	if opts, _ := ParsePageOptions(-2, ""); opts.MaxPages != 1 {
		t.Fatalf("negative max pages: %d", opts.MaxPages)
	}
}

func TestParsePageOptionsSince(t *testing.T) {
	opts, err := ParsePageOptions(3, "2026-10-13T00:00:00+13:00")
	if err != nil || opts.MaxPages != 3 || opts.Since.Unix() != 1791802800 {
		t.Fatalf("got %+v, %v", opts, err)
	}
}

func TestParsePageOptionsRejectsBadSince(t *testing.T) {
	if _, err := ParsePageOptions(3, "2026-10-13"); err == nil {
		t.Fatal("want error for non-RFC3339 since")
	}
}

func TestShouldStop(t *testing.T) {
	since := time.Date(2026, 10, 13, 0, 0, 0, 0, time.FixedZone("UTC+13", 13*3600))
	opts := PageOptions{MaxPages: 3, Since: since}
	newer := Feed{PublishTime: since.Add(time.Hour).UnixMilli()}
	older := Feed{PublishTime: since.Add(-time.Hour).UnixMilli()}
	unknown := Feed{PublishTime: 0}

	check := func(name string, opts PageOptions, page int, feeds []Feed, wantStop, wantReached bool) {
		t.Helper()
		stop, reached := shouldStop(opts, page, feeds)
		if stop != wantStop || reached != wantReached {
			t.Errorf("%s: got (%v,%v) want (%v,%v)", name, stop, reached, wantStop, wantReached)
		}
	}
	check("all newer, more pages", opts, 1, []Feed{newer}, false, false)
	check("contains older", opts, 1, []Feed{newer, older}, true, true)
	check("max pages", opts, 3, []Feed{newer}, true, false)
	check("no new feeds", opts, 2, nil, true, false)
	check("unknown time ignored", opts, 1, []Feed{unknown}, false, false)
	check("no since", PageOptions{MaxPages: 3}, 1, []Feed{older}, false, false)
	check("single page", PageOptions{MaxPages: 1}, 1, []Feed{newer}, true, false)
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
