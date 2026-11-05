package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	commands "github.com/whxcode/mini-gnet/internal/command"
	"github.com/whxcode/mini-gnet/internal/skill"
	"github.com/whxcode/mini-gnet/views/input"
)

// completionItem 只用于候选展示；确认后技能或 MCP 进入草稿片段，命令仍是普通文本。
type completionItem struct {
	name        string
	description string
	skill       bool
	mcp         bool
}

// refreshCompletions 在输入词变化时重新发现技能，不在流式刷新定时器中反复读取磁盘。
func (m *terminalModel) refreshCompletions() {
	prefix, active := m.session.Draft.CompletionPrefix()
	if !active {
		prefix = ""
	}
	if prefix == m.completionPrefix {
		return
	}

	m.completionPrefix = prefix
	m.completions = nil
	m.completionIndex = 0
	m.completionError = ""
	if prefix == "" {
		return
	}

	// 只有整条输入是命令词时列出本地命令；句子中补全技能和 MCP 引用。
	if strings.TrimSpace(m.session.Draft.Value()) == prefix {
		for _, command := range commands.List() {
			if strings.HasPrefix(string(command.Name), prefix) {
				m.completions = append(m.completions, completionItem{
					name:        string(command.Name),
					description: command.Description,
				})
			}
		}
	}

	mcpFilter := strings.ToLower(input.MCPFilter(prefix))
	for _, server := range m.app.mcpManager.List() {
		if strings.Contains(strings.ToLower(server.Name), mcpFilter) {
			m.completions = append(m.completions, completionItem{
				name:        server.Name,
				description: server.Description,
				mcp:         true,
			})
		}
	}

	summaries, err := skill.Discover(m.app.config.Current().ProjectDir)
	if err != nil {
		m.completionError = err.Error()
		return
	}
	filter := strings.ToLower(input.SkillFilter(prefix))
	for _, summary := range summaries {
		if !strings.Contains(strings.ToLower(summary.Name), filter) {
			continue
		}
		m.completions = append(m.completions, completionItem{
			name:        summary.Name,
			description: summary.Description,
			skill:       true,
		})
	}
}

// completionKey 优先处理候选快捷键，Enter 只确认标签，不同时发送提问。
func (m *terminalModel) completionKey(key tea.KeyPressMsg) bool {
	if len(m.completions) == 0 && m.completionError == "" {
		return false
	}
	switch key.String() {
	case "esc":
		m.completions = nil
		m.completionError = ""
		return true
	case "up", "shift+tab":
		m.completionIndex = max(0, m.completionIndex-1)
		return true
	case "down", "tab":
		if len(m.completions) > 0 {
			m.completionIndex = (m.completionIndex + 1) % len(m.completions)
		}
		return true
	case "enter", "ctrl+m":
		if len(m.completions) == 0 {
			return true
		}
		item := m.completions[m.completionIndex]
		if item.mcp {
			m.session.Draft.CompleteMCP(item.name)
		} else {
			m.session.Draft.Complete(item.name, item.skill)
		}
		m.completions = nil
		m.completionError = ""
		m.completionPrefix, _ = m.session.Draft.CompletionPrefix()
		return true
	}

	return false
}

// completionView 按布局分配的高度显示候选，预留错误和操作提示，选中项始终可见。
func (m terminalModel) completionView(width, height int) string {
	if len(m.completions) == 0 && m.completionError == "" {
		return ""
	}
	var lines []string
	if m.completionError != "" {
		lines = append(lines, workspaceNotice.Render(displayLine(m.completionError, width)))
	}
	pageSize := max(1, height-len(lines)-1)
	start := min(m.completionIndex/pageSize*pageSize, max(0, len(m.completions)-pageSize))
	for index := start; index < min(start+pageSize, len(m.completions)); index++ {
		item := m.completions[index]
		kind := "命令 "
		if item.skill {
			kind = "技能 ◆ "
		} else if item.mcp {
			kind = "MCP ◇ "
		}
		prefix := "  "
		if index == m.completionIndex {
			prefix = "› "
		}
		line := displayLine(prefix+kind+item.name+"  ·  "+item.description, width)
		if index == m.completionIndex {
			line = workspaceTitle.Render(line)
		} else {
			line = workspaceMuted.Render(line)
		}
		lines = append(lines, line)
	}
	selected := min(m.completionIndex+1, len(m.completions))
	footer := fmt.Sprintf("Esc 关闭 · ↑↓/Tab 选择 · Enter 确认  %d/%d", selected, len(m.completions))
	lines = append(lines, workspaceMuted.Render(ansi.Truncate(footer, width, "…")))
	return strings.Join(lines, "\n")
}
