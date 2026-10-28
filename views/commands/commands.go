// Package commands 处理本地斜杠命令。这些操作不发送给模型，也不写入聊天正文。
package commands

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/views/message"
)

type Result struct {
	Handled   bool
	Quit      bool
	New       bool
	SessionID string
	Config    *config.Config
}

type Entry struct{ Name, Description string }

var Entries = []Entry{
	{"/help", "查看内置命令和快捷键"},
	{"/config", "编辑配置，保存后立即生效"},
	{"/resume", "选择并恢复历史会话"},
	{"/new", "开始新会话，保留当前记录"},
	{"/exit", "退出程序"},
}

func HelpText() string {
	var lines []string
	for _, entry := range Entries {
		lines = append(lines, entry.Name+"  "+entry.Description)
	}
	return strings.Join(lines, "\n") + "\n\nEnter 发送 · Shift+Enter / Ctrl+J 换行\n菜单：↑↓ 选择 · Enter 确认 · Esc 返回\n/config：Tab / ↑↓ 切换字段 · Esc 放弃修改\n/resume <会话ID> 也可直接恢复指定会话"
}

func Handle(query string, cfg config.Config, currentID string) (Result, error) {
	r := Result{}
	text := strings.TrimSpace(query)
	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, "\n\r") {
		return r, nil
	}
	fields := strings.Fields(text)
	name := fields[0]
	known := false
	for _, entry := range Entries {
		known = known || name == entry.Name
	}
	if !known {
		// 绝对路径可以是正常提问，不能把 /Users/... 一概当作命令。
		if strings.Contains(strings.TrimPrefix(name, "/"), "/") {
			return r, nil
		}
		return Result{Handled: true}, fmt.Errorf("未知命令 %s，输入 /help 查看可用命令", name)
	}
	r.Handled = true
	if len(fields) > 1 && (name != "/resume" || len(fields) != 2) {
		return r, fmt.Errorf("%s 不接受这些参数", name)
	}
	interactive := term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
	switch name {
	case "/exit":
		r.Quit = true
	case "/new":
		r.New = true
	case "/help":
		if !interactive {
			fmt.Println(HelpText())
			return r, nil
		}
		_, err := tea.NewProgram(newHelp()).Run()
		return r, err
	case "/config":
		if !interactive {
			return r, fmt.Errorf("请在交互终端中使用 /config 编辑配置")
		}
		model, err := tea.NewProgram(newConfig(cfg)).Run()
		if err == nil {
			r.Config = model.(configModel).saved
		}
		return r, err
	case "/resume":
		if len(fields) == 2 {
			r.SessionID = fields[1]
			return r, nil
		}
		if !interactive {
			return r, fmt.Errorf("管道中请使用 /resume <会话ID>；交互终端中可用上下键选择")
		}
		sessions, err := message.List(cfg.ChatsDir)
		if err != nil {
			return r, err
		}
		model, err := tea.NewProgram(newResume(sessions, currentID)).Run()
		if err == nil {
			r.SessionID = model.(resumeModel).selected
		}
		return r, err
	}
	return r, nil
}
