package xiaohongshu

import "testing"

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
