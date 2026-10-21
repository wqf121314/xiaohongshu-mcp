package xiaohongshu

import (
	"regexp"
	"strconv"
)

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
