package conversation

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// 调色板集中管理：正文也按来源配色，过程使用较低亮度，错误保持醒目。
const (
	userColor      = "#86E1B4" // 薄荷绿：用户问题。
	deltaColor     = "#79D4EC" // 青蓝：正在流式生成的正文。
	assistantColor = "#D6BEFF" // 浅紫：模型完整回答。
	processColor   = "#A7B7CE" // 灰蓝：已结束的中间轮次。
	toolColor      = "#C2AC82" // 暗金：工具代码和结果。
	systemColor    = "#A0ACBC" // 灰色：运行提示。
	errorColor     = "#FF9DAB" // 珊瑚红：错误正文。
	mutedColor     = "#8F9BAD" // 次要提示与行数。
)

// roleStyle 返回角色的基本文字样式与图标，标题和正文共用同一套颜色。
func roleStyle(role Role) (lipgloss.Style, string) {
	color := systemColor
	icon := "●"

	switch role {
	case User:
		color = userColor
	case Assistant:
		color = assistantColor
		icon = "✦"
	case Tool:
		color = toolColor
		icon = "⚙"
	case System:
		icon = "◆"
	case Error:
		color = errorColor
		icon = "!"
	}

	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)), icon
}

// bodyStyle 额外区分流式片段和中间轮次；Delta 是事件类型，不改变模型消息的角色。
func bodyStyle(event Event) lipgloss.Style {
	style, _ := roleStyle(event.Role)

	if event.Kind == Delta {
		return style.Foreground(lipgloss.Color(deltaColor))
	}

	if event.Role == Assistant && event.Stage == Process {
		return style.Foreground(lipgloss.Color(processColor))
	}

	return style
}

// eventHeader 显示与正文一致的颜色，并以 delta 标签明确标识正在生成的内容。
func eventHeader(event Event) string {
	_, icon := roleStyle(event.Role)
	label := string(event.Role)

	if event.Kind == Delta {
		icon = "◌"
		label = "delta"
	}

	if event.Name != "" {
		label += " · " + strings.ReplaceAll(cleanText(event.Name), "\n", " ")
	}

	return bodyStyle(event).Bold(true).Render(icon + " " + label)
}
