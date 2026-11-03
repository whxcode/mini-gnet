package conversation

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Transcript 是本次排版结果；只裁剪显示区域，原始消息仍完整保留在会话中。
type Transcript struct {
	Content string // 可直接显示的正文、滚动条及行数提示。
	Total   int    // 折行后的总行数。
	Height  int    // 正文可见行数，不包含底部位置提示。
	Offset  int    // 从末尾向上偏移的行数，0 表示跟随最新输出。
}

// MaxOffset 返回能够滚动到第一行的最大偏移，防止越界后需要反向滚动许多次。
func (t Transcript) MaxOffset() int {
	return max(0, t.Total-t.Height)
}

// transcriptLines 按终端宽度排版全部消息，完整保留工具输出和正在生成的文字。
func transcriptLines(messages []Event, stream string, width int) []string {
	var parts []string

	for _, event := range messages {
		parts = append(parts, strings.TrimRight(Format(event, width), "\n"))
	}

	if stream != "" {
		parts = append(parts, strings.TrimRight(Format(Event{
			Kind: Delta,
			Role: Assistant,
			Name: "正在生成 · 尚未结束",
			Text: stream,
		}, width), "\n"))
	}

	if len(parts) == 0 {
		parts = append(parts, "输入问题开始任务；执行期间可用 /new、/sessions 切换会话。")
	}

	return strings.Split(ansi.Hardwrap(strings.Join(parts, "\n\n"), width, true), "\n")
}

// NewTranscript 短内容自然撑开，长内容显示滚动条；为滚动条和行数提示预留空间。
func NewTranscript(messages []Event, stream string, width, height, offset int) Transcript {
	width = max(4, width)
	lines := transcriptLines(messages, stream, width-2)

	return transcriptWindow(lines, width, height, offset)
}

// transcriptWindow 对已排版的正文统一滚动；调用者为右侧位置条预留两列。
func transcriptWindow(lines []string, width, height, offset int) Transcript {
	height = max(1, height)

	if len(lines) <= height {
		return Transcript{
			Content: strings.Join(lines, "\n"),
			Total:   len(lines),
			Height:  height,
		}
	}

	// 溢出时右侧占两列；窗口足够高时，底部再留一行显示位置。
	textWidth := width - 2
	showPosition := height >= 3

	if showPosition {
		height--
	}

	result := Transcript{
		Total:  len(lines),
		Height: height,
	}
	result.Offset = min(max(0, offset), result.MaxOffset())
	end := result.Total - result.Offset
	start := max(0, end-height)
	thumbHeight := max(1, height*height/result.Total)
	thumbStart := start * (height - thumbHeight) / result.MaxOffset()
	visible := make([]string, 0, height+1)

	for row, line := range lines[start:end] {
		marker := "│"

		if row >= thumbStart && row < thumbStart+thumbHeight {
			marker = "┃"
		}

		visible = append(visible, line+strings.Repeat(" ", max(0, textWidth-ansi.StringWidth(line)))+" "+marker)
	}

	if showPosition {
		position := fmt.Sprintf("%d–%d / %d 行 · 滚轮/PgUp/PgDn", start+1, end, result.Total)

		if result.Offset > 0 {
			position += " · Ctrl+End 回到最新"
		}

		visible = append(visible, ansi.Truncate(position, width, "…"))
	}

	result.Content = strings.Join(visible, "\n")

	return result
}
