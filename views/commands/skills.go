package commands

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/internal/skill"
)

// SelectSkill 选择下一条提问使用的技能；空字符串表示 Esc，--clear 表示取消待选。
func SelectSkill(summaries []skill.Summary, current string) (string, error) {
	model, err := tea.NewProgram(newSkills(summaries, current)).Run()
	if err != nil {
		return "", err
	}

	return model.(skillsModel).selected, nil
}

// skillsModel 只处理列表选择，不加载正文或操作会话。
type skillsModel struct {
	summaries []skill.Summary
	current   string
	cursor    int // 0 是取消待选项，其余位置对应 summaries。
	width     int
	height    int
	selected  string
	done      bool
}

// newSkills 复用当前待选位置；没有待选时优先定位第一个技能。
func newSkills(summaries []skill.Summary, current string) skillsModel {
	m := skillsModel{
		summaries: summaries,
		current:   current,
		width:     78,
		height:    24,
	}
	if len(summaries) > 0 {
		m.cursor = 1
	}
	for i, summary := range summaries {
		if summary.Name == current {
			m.cursor = i + 1
			break
		}
	}

	return m
}

// Init 技能列表由命令加载，不需要后台请求。
func (m skillsModel) Init() tea.Cmd {
	return nil
}

// Update 处理选择和翻页；Esc、Ctrl+C 不提交选择，Enter 才确认。
func (m skillsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(10, msg.Width-1)
		m.height = max(8, msg.Height)
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.done = true
			return m, tea.Quit
		case "up", "k":
			m.cursor--
		case "down", "j":
			m.cursor++
		case "pgup":
			m.cursor -= max(1, (m.height-5)/3)
		case "pgdown":
			m.cursor += max(1, (m.height-5)/3)
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = len(m.summaries)
		case "enter":
			m.selected = "--clear"
			if m.cursor > 0 {
				m.selected = m.summaries[m.cursor-1].Name
			}
			m.done = true
			return m, tea.Quit
		}
	}
	m.cursor = min(max(0, m.cursor), len(m.summaries))

	return m, nil
}

// View 按窗口高度分页，名称和简介分行显示，返回提示始终可见。
func (m skillsModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	width := m.width - 2
	lines := []string{accent.Render(clip("选择技能 · 用于下一条提问", width)), ""}
	pageSize := max(1, (m.height-5)/3)
	start := m.cursor / pageSize * pageSize
	end := min(start+pageSize, len(m.summaries)+1)

	for index := start; index < end; index++ {
		name := "取消待选技能"
		description := "保留历史中的技能内容；使用 /new 开始干净会话。"
		if index > 0 {
			summary := m.summaries[index-1]
			name = summary.Name
			description = summary.Description
			if name == m.current {
				name += " · 已选"
			}
		} else if len(m.summaries) == 0 {
			description = "当前 .agent/skills 下没有可选技能。"
		}

		prefix := "  "
		if index == m.cursor {
			prefix = "› "
		}
		title := clip(prefix+name, width)
		if index == m.cursor {
			title = accent.Render(title)
		}
		lines = append(lines, title, muted.Render(clip("  "+description, width)), "")
	}

	footer := fmt.Sprintf("%d/%d · ↑↓ 选择 · Enter 确认 · Esc 返回", m.cursor+1, len(m.summaries)+1)
	if width < 45 {
		footer = "Esc 返回 · ↑↓ · Enter"
	}
	lines = append(lines, muted.Render(clip(footer, width)))
	return tea.NewView(panel.Width(m.width).Render(strings.Join(lines, "\n")))
}
