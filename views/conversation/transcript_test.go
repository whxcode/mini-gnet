package conversation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestTranscriptCanReachEveryLine 验证长工具输出每一行都能通过滚动查看，宽高不越界。
func TestTranscriptCanReachEveryLine(t *testing.T) {
	var lines []string

	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("output-%03d", i))
	}

	messages := []Event{
		{
			Role: Tool,
			Text: strings.Join(lines, "\n"),
		},
	}

	for _, width := range []int{32, 80, 120} {
		bottom := NewTranscript(messages, "", width, 12, 0)

		if !strings.Contains(bottom.Content, "output-099") || bottom.Offset != 0 {
			t.Fatal("默认没有显示最新输出")
		}

		var seen strings.Builder

		for offset := 0; offset <= bottom.MaxOffset(); offset++ {
			view := NewTranscript(messages, "", width, 12, offset)
			seen.WriteString(ansi.Strip(view.Content))

			if len(strings.Split(view.Content, "\n")) > 12 {
				t.Fatal("正文高度溢出")
			}

			for _, line := range strings.Split(view.Content, "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatal("正文或滚动条宽度溢出")
				}
			}
		}

		for _, line := range lines {
			if !strings.Contains(seen.String(), line) {
				t.Fatalf("无法查看完整输出，缺少 %s", line)
			}
		}

		top := NewTranscript(messages, "", width, 12, 10000)

		if !strings.Contains(top.Content, "output-000") || top.Offset != top.MaxOffset() {
			t.Fatal("没有正确限制顶部偏移")
		}
	}
}
