package xiaohongshu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeTopic(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"去首尾空白与井号", " #奥克兰 ", "奥克兰"},
		{"去多个开头井号", "##奥克兰", "奥克兰"},
		{"去结尾话题标记与井号", "奥克兰[话题]#", "奥克兰"},
		{"只有结尾话题标记", "奥克兰[话题]", "奥克兰"},
		{"首尾都有井号", "#奥克兰#", "奥克兰"},
		{"全角空格", "　#奥克兰　", "奥克兰"},
		{"无需处理的原样返回", "奥克兰二手", "奥克兰二手"},
		{"中间的井号保留", "C#入门", "C#入门"},
		{"重复标记也能收敛", "#奥克兰[话题][话题]#", "奥克兰"},
		{"空串", "", ""},
		{"只有井号", "###", ""},
		{"只有话题标记", "[话题]", ""},
		{"只有空白", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeTopic(tt.in)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, got, NormalizeTopic(got), "重复规范化结果应不变")
		})
	}
}

func TestPickTopic(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		options []string
		idx     int
	}{
		{"命中带井号的同名项", "奥克兰二手", []string{"奥克兰二手车", "#奥克兰二手", "奥克兰"}, 1},
		{"want 带井号也能命中", "#奥克兰", []string{"奥克兰二手车", "奥克兰[话题]#"}, 1},
		{"多个同名取第一个", "奥克兰", []string{"x", "奥克兰", "#奥克兰"}, 1},
		{"全不同名", "奥克兰", []string{"奥克兰二手车", "新西兰"}, -1},
		{"空列表", "奥克兰", []string{}, -1},
		{"nil 列表", "奥克兰", nil, -1},
		{"want 规范化后为空", "##", []string{"", "#"}, -1},
		{"区分大小写", "ipad", []string{"iPad"}, -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.idx, PickTopic(tt.want, tt.options))
		})
	}
}

func TestFormatTopicResults(t *testing.T) {
	t.Run("已关联与未关联混合", func(t *testing.T) {
		got := FormatTopicResults([]TopicResult{
			{Name: "奥克兰", Linked: true},
			{Name: "xx", Linked: false},
		})
		assert.Equal(t, "话题：#奥克兰 ✅已关联；#xx ⚠️未找到同名话题，按普通文字发出", got)
	})

	t.Run("单个结果", func(t *testing.T) {
		got := FormatTopicResults([]TopicResult{{Name: "奥克兰", Linked: true}})
		assert.Equal(t, "话题：#奥克兰 ✅已关联", got)
	})

	t.Run("nil 返回空串", func(t *testing.T) {
		assert.Equal(t, "", FormatTopicResults(nil))
	})

	t.Run("空切片返回空串", func(t *testing.T) {
		assert.Equal(t, "", FormatTopicResults([]TopicResult{}))
	})
}

func TestParseViewCount(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"1.2亿次浏览", 120000000},
		{"3.4万", 34000},
		{"856次浏览", 856},
		{"856", 856},
		{"1亿", 100000000},
		{"2万次浏览", 20000},
		{"3.4w", 34000},
		{"3.4W", 34000},
		{" 3.4 万 浏览 ", 34000},
		{"12浏览", 12},
		{"12次 浏览", 12},
		{"1.23456万", 12346},
		{"7次", 7},
		{"0", 0},
		{"", 0},
		{"abc", 0},
		{"万", 0},
		{"-5万", 0},
		{"1.2.3万", 0},
		{"inf", 0},
		{"NaN", 0},
		{"99999999999999999999亿", 0},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			assert.Equal(t, tt.want, ParseViewCount(tt.in))
		})
	}
}
