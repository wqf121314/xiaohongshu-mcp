package xiaohongshu

import (
	"strconv"
	"strings"
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

func TestParseTopicSearchResponse(t *testing.T) {
	sample := `{"success":true,"code":0,"msg":"成功","data":{"topics":[
		{"topic_info_dto":{"name":"奥克兰","view_num":948369929,"discuss_num_desc":"9.5亿浏览","id":"a1","link":"https://x/a1","hot_topic":false}},
		{"topic_info_dto":{"name":"奥克兰生活","view_num":123456789,"discuss_num_desc":"1.2亿浏览","id":"a2","link":"https://x/a2","hot_topic":true}},
		{"topic_info_dto":{"name":"奥克兰二手","view_num":35000,"discuss_num_desc":"3.5万浏览","id":"a3","link":"https://x/a3","hot_topic":false}}
	]}}`

	tests := []struct {
		name    string
		body    string
		want    []TopicInfo
		wantErr bool
	}{
		{"正常返回", sample, []TopicInfo{
			{Name: "奥克兰", Views: 948369929, ViewsText: "9.5亿浏览"},
			{Name: "奥克兰生活", Views: 123456789, ViewsText: "1.2亿浏览"},
			{Name: "奥克兰二手", Views: 35000, ViewsText: "3.5万浏览"},
		}, false},
		{"空列表", `{"success":true,"code":0,"data":{"topics":[]}}`, []TopicInfo{}, false},
		{"没有 topics 字段", `{"success":true,"code":0,"data":{}}`, []TopicInfo{}, false},
		{"非法 JSON", `{"success":tru`, nil, true},
		{"空 body", ``, nil, true},
		{"success 为 false", `{"success":false,"code":-1,"msg":"签名错误"}`, nil, true},
		{"code 非 0", `{"success":true,"code":500,"data":{"topics":[]}}`, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTopicSearchResponse([]byte(tt.body))
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTopicSearchKeyword(t *testing.T) {
	tests := []struct {
		name     string
		postData string
		want     string
		wantOK   bool
	}{
		{"正常请求体", `{"keyword":"奥克兰","topic_round_start_time":1760500000000,"content":""}`, "奥克兰", true},
		{"关键词为空", `{"keyword":"","content":""}`, "", true},
		{"没有关键词字段", `{"content":""}`, "", false},
		{"空请求体", ``, "", false},
		{"非法 JSON", `keyword=奥克兰`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := topicSearchKeyword(tt.postData)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestMatchDropdownItem(t *testing.T) {
	tests := []struct {
		name      string
		itemText  string
		topic     string
		viewsText string
		want      bool
	}{
		{"带井号和浏览量", "#奥克兰 9.5亿浏览", "奥克兰", "9.5亿浏览", true},
		{"名字与浏览量换行分开", "#奥克兰\n9.5亿浏览", "奥克兰", "9.5亿浏览", true},
		{"只有名字", "奥克兰", "奥克兰", "", true},
		{"只有空白与井号差异", "  # 奥克兰 ", "奥克兰", "9.5亿浏览", true},
		{"名字含空格", "#New Zealand 1.2亿浏览", "New Zealand", "1.2亿浏览", true},
		{"更长的名字不算", "#奥克兰二手车 3.5万浏览", "奥克兰二手", "3.5万浏览", false},
		{"更短的名字不算", "#奥克兰 9.5亿浏览", "奥克兰二手", "9.5亿浏览", false},
		{"多出标记文字不算", "#奥克兰 热 9.5亿浏览", "奥克兰", "9.5亿浏览", false},
		{"浏览量写法不同不算", "#奥克兰 9.5亿次浏览", "奥克兰", "9.5亿浏览", false},
		{"空文字", "", "奥克兰", "9.5亿浏览", false},
		{"空话题名", "#", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MatchDropdownItem(tt.itemText, tt.topic, tt.viewsText))
		})
	}
}

func TestTagsLimit(t *testing.T) {
	ten := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	assert.NoError(t, checkTagsLimit(nil))
	assert.NoError(t, checkTagsLimit([]string{"奥克兰"}))
	assert.NoError(t, checkTagsLimit(ten))

	err := checkTagsLimit(append(ten, "11"))
	if assert.Error(t, err) {
		assert.Equal(t, "话题最多 10 个", err.Error())
	}
}

func TestFormatTopicList(t *testing.T) {
	many := make([]TopicInfo, 25)
	for i := range many {
		many[i] = TopicInfo{Name: "话题" + strconv.Itoa(i+1), ViewsText: "1万浏览"}
	}
	manyLines := make([]string, 20)
	for i := range manyLines {
		manyLines[i] = "#话题" + strconv.Itoa(i+1) + "（1万浏览）"
	}

	tests := []struct {
		name string
		ts   []TopicInfo
		max  int
		want string
	}{
		{
			name: "正常",
			ts: []TopicInfo{
				{Name: "奥克兰", Views: 950000000, ViewsText: "9.5亿浏览"},
				{Name: "奥克兰二手", Views: 12000, ViewsText: "1.2万浏览"},
			},
			max:  20,
			want: "#奥克兰（9.5亿浏览）\n#奥克兰二手（1.2万浏览）",
		},
		{name: "nil", ts: nil, max: 20, want: "没有找到相关话题"},
		{name: "空切片", ts: []TopicInfo{}, max: 20, want: "没有找到相关话题"},
		{name: "超过上限截断", ts: many, max: 20, want: strings.Join(manyLines, "\n")},
		{
			name: "浏览量文字为空",
			ts:   []TopicInfo{{Name: "奥克兰"}, {Name: "奥克兰租房", ViewsText: "3万浏览"}},
			max:  20,
			want: "#奥克兰\n#奥克兰租房（3万浏览）",
		},
		{name: "max 不大于 0 不限制", ts: many[:3], max: 0, want: "#话题1（1万浏览）\n#话题2（1万浏览）\n#话题3（1万浏览）"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatTopicList(tt.ts, tt.max))
		})
	}
}
