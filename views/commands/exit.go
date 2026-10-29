package commands

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
)

type exitModel struct {
	running         int
	done, confirmed bool
}

// Init 退出确认不启动后台操作，已有任务仍独立运行。
func (m exitModel) Init() tea.Cmd { return nil }

// Update 仅在 Enter 时确认退出；Esc 或 Ctrl+C 返回输入界面。
func (m exitModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "enter":
			m.confirmed, m.done = true, true
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// View 说明退出会停止任务，避免把后台误认为进程退出后仍能运行。
func (m exitModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(panel.Render(fmt.Sprintf("还有 %d 个任务正在执行。\nEnter 停止所有任务并退出 · Esc 返回", m.running)))
}
