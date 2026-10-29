// Package conversation 负责对话显示。Agent 只发送事件，不直接操作终端光标。
package conversation

import (
	"fmt"
	"os/user"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
	Tool      Role = "tool"
	System    Role = "system"
	Error     Role = "error"
)

type Kind int

const (
	Message Kind = iota // 完整消息，放入上方的对话记录。
	Status              // 等待模型、执行工具等状态，更新底部标题。
	Delta               // 模型新生成的文本片段，只更新底部预览。
)

type Event struct {
	Kind Kind
	Role Role
	Name string // 用户名、模型名或工具名。
	Text string
}

// Username 返回用于消息标题的系统用户名，读取失败时使用“你”。
func Username() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "你"
}

// roleStyle 为各消息角色选择颜色和图标。
func roleStyle(role Role) (lipgloss.Style, string) {
	color := "#A7C7F5"
	icon := "●"
	switch role {
	case User:
		color = "#9BC5AE"
	case Assistant:
		color = "#B9A7DE"
		icon = "✦"
	case Tool:
		color = "#D9BC83"
		icon = "⚙"
	case System:
		color = "#9ABEC9"
		icon = "◆"
	case Error:
		color = "#E3A0A0"
		icon = "!"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)), icon
}

// cleanText 去掉外部输出里的终端控制序列，避免工具输出改变界面或移动光标。
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

// Header 生成安全的角色标题，去掉名称中的终端控制字符。
func Header(role Role, name string) string {
	style, icon := roleStyle(role)
	label := string(role)
	if name != "" {
		label += " · " + strings.ReplaceAll(cleanText(name), "\n", " ")
	}
	return style.Bold(true).Render(icon + " " + label)
}

// Format 用彩色标题区分角色，正文保持浅色并缩进，避免整段高饱和颜色影响阅读。
func Format(e Event, width int) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#D8DEE9"))
	body := strings.TrimRight(cleanText(e.Text), "\n")
	if width > 0 {
		body = ansi.Hardwrap(body, max(1, width-3), true)
	}
	return Header(e.Role, e.Name) + "\n" + style.Render("  "+strings.ReplaceAll(body, "\n", "\n  ")) + "\n"
}

// Print 用于普通日志或非交互管道；流式终端界面使用自己的事件队列。
func Print(e Event) {
	if e.Kind == Message {
		fmt.Println(Format(e, 0))
	}
}
