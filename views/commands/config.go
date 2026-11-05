package commands

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/config"
)

var fieldLabels = []string{
	"接口地址 Base URL",
	"模型名称",
	"API Key（隐藏显示）",
	"聊天记录目录",
	"工作目录（留空使用启动目录）",
	"工具目录（默认 my_tools，修改后重启生效）",
	"上下文上限 tokens（仅供统计，0 表示未知）",
	"MCP 配置文件（修改后重启生效）",
}

type configModel struct {
	fields []textinput.Model
	focus  int
	width  int
	height int
	saved  *config.Config
	done   bool
	err    string
}

// newConfig 创建配置副本，只有确认保存时才交回调用方。
func newConfig(cfg config.Config) configModel {
	values := []string{
		cfg.BaseURL,
		cfg.Model,
		cfg.APIKey,
		cfg.ChatsDir,
		cfg.ProjectDir,
		cfg.ToolsDir,
		strconv.Itoa(cfg.ContextWindow),
		cfg.MCPConfigPath,
	}
	m := configModel{width: 78, height: 24}

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

// Init 配置表单无需异步加载。
func (m configModel) Init() tea.Cmd {
	return nil
}

// Update 处理字段编辑、校验和 Esc 放弃，不直接保存配置。
func (m configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(10, msg.Width-1)
		m.height = max(5, msg.Height)

		for i := range m.fields {
			position := m.fields[i].Position()
			m.fields[i].SetWidth(max(1, m.width-3))
			m.fields[i].CursorEnd()
			m.fields[i].SetCursor(position)
		}

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			// 编辑的是副本，Esc 直接丢弃它，不写文件、不改变当前客户端。
			m.done = true

			return m, tea.Quit

		case "tab", "down", "shift+tab", "up", "enter":
			if msg.String() == "enter" && m.focus == len(m.fields) {
				contextWindow, err := strconv.Atoi(strings.TrimSpace(m.fields[6].Value()))
				if err != nil || contextWindow < 0 {
					m.err = "上下文上限必须是非负整数；未知时填写 0"
					return m, nil
				}

				cfg, err := config.Normalize(config.Config{
					BaseURL:       m.fields[0].Value(),
					Model:         m.fields[1].Value(),
					APIKey:        m.fields[2].Value(),
					ChatsDir:      m.fields[3].Value(),
					ProjectDir:    m.fields[4].Value(),
					ToolsDir:      m.fields[5].Value(),
					ContextWindow: contextWindow,
					MCPConfigPath: m.fields[7].Value(),
				})
				if err != nil {
					m.err = err.Error()

					return m, nil
				}

				m.saved = &cfg
				m.done = true

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

// View 按可用高度显示字段，短窗口聚焦当前项并保留错误与返回提示。
func (m configModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	lines := []string{accent.Render("◆ 配置")}
	compact := m.height < 2*len(m.fields)+6
	if !compact {
		lines = append(lines, "")
	}

	cursorLine := 0

	for i, field := range m.fields {
		if compact && i != m.focus {
			continue
		}

		label := fieldLabels[i]

		if i == m.focus {
			label = accent.Render(clip("› "+label, m.width-2))
		} else {
			label = muted.Render(clip("  "+label, m.width-2))
		}

		if i == m.focus {
			cursorLine = len(lines) + 1
		}

		lines = append(lines, label, field.View())
	}

	button := "  保存并应用"

	if m.focus == len(m.fields) {
		button = accent.Render("› 保存并应用")
	}

	if !compact {
		lines = append(lines, "")
	}
	if !compact || m.focus == len(m.fields) {
		lines = append(lines, button)
	}

	if m.err != "" {
		lines = append(lines, bad.Render(clip(m.err, m.width-2)))
	}

	lines = append(lines, muted.Render(clip("Esc 返回 · Tab / ↑↓ 切换 · Enter 下一项/保存", m.width-2)))
	v := tea.NewView(panel.Width(m.width).Render(strings.Join(lines, "\n")))

	if m.focus < len(m.fields) {
		v.Cursor = m.fields[m.focus].Cursor()

		if v.Cursor != nil {
			v.Cursor.Position.X++
			v.Cursor.Position.Y += cursorLine
		}
	}

	return v
}
