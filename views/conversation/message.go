// Package conversation 负责对话显示。Agent 只发送事件，不直接操作终端光标。
package conversation

import (
	"fmt"
	"os/user"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/internal/agent"
)

// 显示层使用 Agent 定义的输出类型，核心包不依赖终端组件。
type Role = agent.Role

type Kind = agent.Kind

type Event = agent.Event

const (
	Process = agent.Process
	Final   = agent.Final
)

const (
	User      = agent.User
	Assistant = agent.Assistant
	Tool      = agent.Tool
	System    = agent.System
	Error     = agent.Error
	Message   = agent.Message
	Status    = agent.Status
	Delta     = agent.Delta
)

// Username 返回用于消息标题的系统用户名，读取失败时使用“你”。
func Username() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}

	return "你"
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
	return eventHeader(Event{
		Role: role,
		Name: name,
	})
}

// Format 按来源为标题和整段正文配色；标题加粗、正文缩进，长文本也能区分层次。
func Format(e Event, width int) string {
	if e.Role == User && e.Name == "" {
		e.Name = Username()
	}

	if e.Role == Assistant && e.Stage == Final {
		e.Name = "最终回答"

		if e.Text == "" {
			e.Text = "模型已结束本次回答，未返回正文。"
		}
	}

	if e.Role == Assistant && e.Stage == Process && e.Name == "" {
		e.Name = "中间轮次"
	}

	style := bodyStyle(e)
	body := strings.TrimRight(cleanText(e.Text), "\n")

	if width > 0 {
		body = ansi.Hardwrap(body, max(1, width-3), true)
	}

	return eventHeader(e) + "\n" + style.Render("  "+strings.ReplaceAll(body, "\n", "\n  ")) + "\n"
}

// Print 用于普通日志或非交互管道；流式终端界面使用自己的事件队列。
func Print(e Event) {
	if e.Kind == Message {
		fmt.Println(Format(e, 0))
	}
}
