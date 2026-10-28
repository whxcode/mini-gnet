package commands

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/config"
)

var fieldLabels = []string{"接口地址 Base URL", "模型名称", "API Key（隐藏显示）", "聊天记录目录", "工作目录（留空使用启动目录）"}

type configModel struct {
	fields []textinput.Model
	focus  int
	width  int
	saved  *config.Config
	done   bool
	err    string
}

func newConfig(cfg config.Config) configModel {
	values := []string{cfg.BaseURL, cfg.Model, cfg.APIKey, cfg.ChatsDir, cfg.ProjectDir}
	m := configModel{width: 78}
	for i, value := range values {
		field := textinput.New()
		field.Prompt = ""
		field.SetWidth(74)
		field.SetVirtualCursor(false)
		field.SetValue(value)
		if i == 2 {
			field.EchoMode = textinput.EchoPassword
			field.EchoCharacter = '•'
		}
		m.fields = append(m.fields, field)
	}
	m.fields[0].Focus()
	return m
}

func (m configModel) Init() tea.Cmd { return nil }
func (m configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(10, msg.Width-1)
		for i := range m.fields {
			m.fields[i].SetWidth(max(4, m.width-4))
		}
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			// 编辑的是副本，Esc 直接丢弃它，不写文件、不改变当前客户端。
			m.done = true
			return m, tea.Quit
		case "tab", "down", "shift+tab", "up", "enter":
			if msg.String() == "enter" && m.focus == len(m.fields) {
				cfg, err := config.Normalize(config.Config{
					BaseURL: m.fields[0].Value(), Model: m.fields[1].Value(), APIKey: m.fields[2].Value(),
					ChatsDir: m.fields[3].Value(), ProjectDir: m.fields[4].Value(),
				})
				if err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.saved, m.done = &cfg, true
				return m, tea.Quit
			}
			if m.focus < len(m.fields) {
				m.fields[m.focus].Blur()
			}
			delta := 1
			if msg.String() == "up" || msg.String() == "shift+tab" {
				delta = -1
			}
			m.focus = (m.focus + delta + len(m.fields) + 1) % (len(m.fields) + 1)
			if m.focus < len(m.fields) {
				return m, m.fields[m.focus].Focus()
			}
			return m, nil
		}
	}
	if m.focus < len(m.fields) {
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m configModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	lines := []string{accent.Render("◆ 配置"), ""}
	for i, field := range m.fields {
		label := fieldLabels[i]
		if i == m.focus {
			label = accent.Render("› " + label)
		} else {
			label = muted.Render("  " + label)
		}
		lines = append(lines, label, field.View())
	}
	button := "  保存并应用"
	if m.focus == len(m.fields) {
		button = accent.Render("› 保存并应用")
	}
	lines = append(lines, "", button)
	if m.err != "" {
		lines = append(lines, bad.Render(clip(m.err, m.width-2)))
	}
	lines = append(lines, muted.Render("↑↓ / Tab 切换 · Enter 下一项/保存 · Esc 放弃并返回"))
	v := tea.NewView(panel.Width(m.width).Render(strings.Join(lines, "\n")))
	if m.focus < len(m.fields) {
		v.Cursor = m.fields[m.focus].Cursor()
		if v.Cursor != nil {
			v.Cursor.Position.X++
			v.Cursor.Position.Y += 3 + m.focus*2
		}
	}
	return v
}
