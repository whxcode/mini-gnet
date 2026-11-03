package conversation

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestProcessPreviewAndFinalAnswer 验证多轮过程折叠、完整结果可查、最终正文只出现一次。
func TestProcessPreviewAndFinalAnswer(t *testing.T) {
	messages := []Event{
		{
			Role: User,
			Text: "用户问题",
		},
		{
			Role:  Assistant,
			Stage: Process,
			Text:  "中间分析不能冒充最终回答",
		},
		{
			Role: Tool,
			Name: "run_python · 结果",
			Text: "工具开头\n" + strings.Repeat("过程行\n", 80) + "工具结尾",
		},
		{
			Role:  Assistant,
			Stage: Final,
			Text:  "唯一最终回答",
		},
	}

	for _, width := range []int{32, 80, 120} {
		chat := NewChatTranscript(messages, "", "", width, 100, 0)
		plain := ansi.Strip(chat.Content)

		if strings.Contains(plain, "工具开头") ||
			!strings.Contains(plain, "工具结尾") ||
			strings.Count(plain, "唯一最终回答") != 1 ||
			!strings.Contains(plain, "已结束") {
			t.Fatalf("折叠或最终回答不正确：\n%s", plain)
		}

		if len(strings.Split(plain, "\n")) > 24 {
			t.Fatal("过程预览没有限制高度")
		}

		for _, line := range strings.Split(plain, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("过程边框撑破终端宽度")
			}
		}
	}

	full := NewTranscript(messages, "", 80, 200, 0)

	if !strings.Contains(full.Content, "工具开头") ||
		!strings.Contains(full.Content, "中间分析不能冒充最终回答") {
		t.Fatal("折叠丢失完整过程")
	}

	live := NewChatTranscript(messages[:1], "候选正文", "正在生成", 80, 30, 0)

	if strings.Contains(live.Content, "最终回答") ||
		!strings.Contains(live.Content, "进行中") ||
		!strings.Contains(live.Content, "候选正文") {
		t.Fatal("流式正文被提前标为最终回答")
	}
}
