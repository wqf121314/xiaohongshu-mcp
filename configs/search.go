package configs

import "fmt"

// DefaultMaxSearchPages 单次搜索最多翻几页的默认值。
const DefaultMaxSearchPages = 5

// 单次搜索最多翻几页（第一屏算 1 页），启动时设定。
var maxSearchPages = DefaultMaxSearchPages

// SetMaxSearchPages 设定翻页上限；小于 1 时返回错误，上限保持原值。
func SetMaxSearchPages(n int) error {
	if n < 1 {
		return fmt.Errorf("-max-search-pages 至少为 1，收到 %d", n)
	}
	maxSearchPages = n
	return nil
}

// MaxSearchPages 返回当前的翻页上限。
func MaxSearchPages() int {
	return maxSearchPages
}
