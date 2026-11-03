package conversation

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// isProcess 根据 Agent 的标记归类，不根据正文用词判断是否完成。
func isProcess(event Event) bool {
	return event.Stage == Process || event.Role == Tool
}

// processPanel 只展示最近六行过程预览，完整记录仍可在详情中滚动查看。
func processPanel(events []Event, status string, width int) string {
	innerWidth := max(1, width-4)
	lines := transcriptLines(events, "", innerWidth)
	preview := lines[max(0, len(lines)-6):]
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(processColor)).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(mutedColor))
	text := titleStyle.Render("执行过程 · "+status) + "\n" + strings.Join(preview, "\n")
	text += "\n" + hintStyle.Render(fmt.Sprintf("共 %d 行 · Ctrl+O 查看完整记录", len(lines)))

	// 先折行再加边框，窄窗口里的中文提示也不能撑破布局。
	text = ansi.Hardwrap(text, innerWidth, true)

	// Width 包含边框和内边距，让短过程也占满正文可用宽度。
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#53617A")).
		Padding(0, 1).
		Render(text)
}

// NewChatTranscript 按“提问、执行过程、最终回答”排版；流式正文在收到 stop 前放在过程内。
func NewChatTranscript(messages []Event, stream, progress string, width, height, offset int) Transcript {
	width = max(8, width)
	textWidth := width - 2
	events := append([]Event(nil), messages...)
	finished := len(events) > 0 && (events[len(events)-1].Stage == Final || events[len(events)-1].Role == Error)

	if !finished && (stream != "" || progress != "") {
		live := Event{
			Kind:  Delta,
			Role:  Assistant,
			Name:  "正在生成 · 尚未结束",
			Text:  stream,
			Stage: Process,
		}

		if stream == "" {
			// 等待模型或执行工具只有状态提示，不能冒充模型正在输出的正文。
			live = Event{
				Role:  System,
				Name:  "执行状态",
				Text:  progress,
				Stage: Process,
			}
		}

		events = append(events, live)
	}

	var parts []string

	for i := 0; i < len(events); {
		if !isProcess(events[i]) {
			parts = append(parts, strings.TrimRight(Format(events[i], textWidth), "\n"))
			i++
			continue
		}

		start := i

		for i < len(events) && isProcess(events[i]) {
			i++
		}

		status := "执行记录"

		if i == len(events) && !finished && progress != "" {
			status = "进行中"
		} else if i < len(events) && events[i].Stage == Final {
			status = "已结束"
		} else if i < len(events) && events[i].Role == Error {
			status = "未完成"
		}

		parts = append(parts, processPanel(events[start:i], status, textWidth))
	}

	if len(parts) == 0 {
		parts = append(parts, "输入问题开始任务；Ctrl+O 查看完整执行记录。")
	}

	lines := strings.Split(ansi.Hardwrap(strings.Join(parts, "\n\n"), textWidth, true), "\n")

	return transcriptWindow(lines, width, height, offset)
}
