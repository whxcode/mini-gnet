package commands

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/views/message"
)

var (
	panel  = lipgloss.NewStyle().Background(lipgloss.Color("#303544")).Foreground(lipgloss.Color("#E5E9F0")).Padding(0, 1)
	accent = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A5ADBD"))
	bad    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func clip(s string, width int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
	return ansi.Truncate(s, max(1, width), "…")
}

type helpModel struct {
	width int
	done  bool
}

func newHelp() helpModel          { return helpModel{width: 78} }
func (m helpModel) Init() tea.Cmd { return nil }
func (m helpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(10, msg.Width-1)
	case tea.KeyPressMsg:
		if msg.String() == "esc" || msg.String() == "ctrl+c" || msg.String() == "enter" {
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m helpModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	return tea.NewView(panel.Width(m.width).Render(accent.Render("◆ 内置命令") + "\n\n" + HelpText() + "\n\n" + muted.Render("Esc / Enter 返回输入")))
}

type resumeModel struct {
	sessions []message.SessionInfo
	current  string
	cursor   int
	width    int
	height   int
	selected string
	problem  string
	done     bool
}

func newResume(sessions []message.SessionInfo, current string) resumeModel {
	return resumeModel{sessions: sessions, current: current, width: 78, height: 24}
}
func (m resumeModel) Init() tea.Cmd { return nil }
func (m resumeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(10, msg.Width-1), msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
			m.problem = ""
		case "down", "j":
			m.cursor = max(0, min(len(m.sessions)-1, m.cursor+1))
			m.problem = ""
		case "enter":
			if len(m.sessions) == 0 {
				return m, nil
			}
			s := m.sessions[m.cursor]
			if s.Problem != "" {
				m.problem = s.Problem
				return m, nil
			}
			m.selected, m.done = s.ID, true
			return m, tea.Quit
		}
	}
	return m, nil
}
func (m resumeModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	lines := []string{accent.Render("◆ 恢复会话"), ""}
	if len(m.sessions) == 0 {
		lines = append(lines, "当前聊天记录目录中没有可恢复的会话。")
	} else {
		// 列表分页跟随选择位置，避免大量会话把菜单撑出屏幕。
		pageSize := max(1, (m.height-7)/3)
		start := (m.cursor / pageSize) * pageSize
		for i := start; i < min(start+pageSize, len(m.sessions)); i++ {
			s := m.sessions[i]
			prefix := "  "
			if i == m.cursor {
				prefix = "› "
			}
			title := prefix + s.Title
			if s.ID == m.current {
				title += " [当前]"
			}
			if s.Problem != "" {
				title += " [损坏]"
			}
			title = clip(title, m.width-2)
			if i == m.cursor {
				title = accent.Render(title)
			}
			meta := fmt.Sprintf("  %s · %d 条消息 · %s", s.UpdatedAt.Format("01-02 15:04"), s.Count, s.ID)
			lines = append(lines, title, muted.Render(clip(meta, m.width-2)), "")
		}
		lines = append(lines, fmt.Sprintf("%d / %d", m.cursor+1, len(m.sessions)))
	}
	if m.problem != "" {
		lines = append(lines, bad.Render(clip(m.problem, m.width-2)))
	}
	lines = append(lines, muted.Render("↑↓ 选择 · Enter 恢复 · Esc 返回"))
	return tea.NewView(panel.Width(m.width).Render(strings.Join(lines, "\n")))
}
