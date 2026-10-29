package app

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/views/conversation"
)

type refreshMsg struct{}

// terminalModel 只编辑当前会话并刷新快照。提交后把输入交回 App.Run，命令 handler 仍直接执行。
type terminalModel struct {
	app        *App
	session    *Session
	ctx        context.Context
	width      int
	height     int
	query      string
	fromEditor bool
	done       bool
	err        error
}

// 背景保持连续，输入框略亮；颜色强调标题，正文和操作提示降低视觉噪声。
var (
	workspacePanel  = lipgloss.NewStyle().Background(lipgloss.Color("#262B36")).Foreground(lipgloss.Color("#D8DEE9")).Padding(1, 2)
	workspaceTitle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#A7C7F5")).Bold(true)
	workspaceMuted  = lipgloss.NewStyle().Foreground(lipgloss.Color("#929DAF"))
	workspaceNotice = lipgloss.NewStyle().Foreground(lipgloss.Color("#D9BC83"))
	composerPanel   = lipgloss.NewStyle().Background(lipgloss.Color("#303544")).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#53617A")).Padding(0, 1)
)

// readInput 由交互主循环调用，等待一次输入；后台任务独立执行。
// 第二个返回值标明是否按 Enter 提交草稿；快捷命令返回 false，让主循环保留未发送的草稿。
func (a *App) readInput(ctx context.Context) (string, bool, error) {
	result, err := tea.NewProgram(newTerminal(a, ctx)).Run()
	if err != nil {
		return "", false, err
	}
	m := result.(terminalModel)
	return m.query, m.fromEditor, m.err
}

// newTerminal 复用会话自己的编辑框，因此切换回来时草稿和光标仍在原位置。
func newTerminal(a *App, ctx context.Context) terminalModel {
	s := a.CurrentSession
	s.markRead()
	s.draft.SetWidth(72) // 第一帧按默认 80 列预留外层留白与输入框边框，随后跟随窗口尺寸。
	return terminalModel{app: a, session: s, ctx: ctx, width: 80, height: 24}
}

// refresh 定时刷新后台状态；任务不依赖这个定时器，打开其他菜单时也继续执行。
func refresh() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return refreshMsg{} })
}

// Init 只启动显示刷新，不通过界面回调启动 Agent。
func (m terminalModel) Init() tea.Cmd { return refresh() }

// submit 只把文本交回主循环；是否执行、是否清空草稿均由主循环决定。
func (m terminalModel) submit(query string) (tea.Model, tea.Cmd) {
	m.query = query
	m.done = true
	return m, tea.Quit
}

// Update 处理编辑和会话快捷键；后台输出只更新显示，绝不会占用或覆盖输入内容。
func (m terminalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case refreshMsg:
		if err := m.ctx.Err(); err != nil {
			m.err = err
			m.done = true
			return m, tea.Quit
		}
		m.session.markRead()
		return m, refresh()
	case tea.WindowSizeMsg:
		m.width = max(12, msg.Width)
		m.height = max(8, msg.Height)
		m.session.draft.MaxHeight = max(1, m.height-16)
		m.session.draft.SetWidth(max(4, m.width-8))
	case tea.KeyPressMsg:
		switch msg.String() {
		case "f2":
			return m.submit("/sessions")
		case "alt+left":
			return m.submit("/sessions prev")
		case "alt+right":
			return m.submit("/sessions next")
		case "alt+n":
			return m.submit("/new")
		case "ctrl+c":
			if m.session.snapshot().Running {
				return m.submit("/stop")
			}
			return m.submit("/exit")
		case "pgup":
			m.session.scrollOffset += max(1, m.height/2)
			return m, nil
		case "pgdown":
			m.session.scrollOffset = max(0, m.session.scrollOffset-max(1, m.height/2))
			return m, nil
		case "enter", "ctrl+m":
			query := m.session.draft.Value()
			if strings.TrimSpace(query) == "" {
				return m, nil
			}
			m.fromEditor = true
			return m.submit(query)
		}
	}
	var cmd tea.Cmd
	m.session.draft, cmd = m.session.draft.Update(msg)
	return m, cmd
}

// displayLine 去掉控制字符并限制为一行，避免模型标题破坏终端布局。
func displayLine(text string, width int) string {
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(text))
	return ansi.Truncate(strings.Join(strings.Fields(text), " "), max(1, width), "…")
}

// transcript 按内容自然增长，仅在超过可见高度时裁剪；不补空行把输入框推到底部。
func transcript(snapshot sessionSnapshot, width, height, offset int) string {
	var parts []string
	for _, event := range snapshot.Messages {
		parts = append(parts, strings.TrimRight(conversation.Format(event, width), "\n"))
	}
	if snapshot.StreamText != "" {
		parts = append(parts, strings.TrimRight(conversation.Format(conversation.Event{Role: conversation.Assistant, Text: snapshot.StreamText}, width), "\n"))
	}
	if len(parts) == 0 {
		parts = append(parts, "输入问题开始任务；执行期间可用 /new、/sessions 切换会话。")
	}
	lines := strings.Split(ansi.Hardwrap(strings.Join(parts, "\n\n"), width, true), "\n")
	offset = min(offset, max(0, len(lines)-height))
	end := len(lines) - offset
	start := max(0, end-height)
	return strings.Join(lines[start:end], "\n")
}

// backgroundNotices 把后台完成状态单独显示，避免长会话标签截断后用户只能切回才发现结果。
func (m terminalModel) backgroundNotices(width, limit int) []string {
	var notices []string
	for i, s := range m.app.opened {
		if s == m.session {
			continue
		}
		state := s.snapshot()
		if state.HasUnreadResult {
			notices = append(notices, displayLine(fmt.Sprintf("◆ 会话 %d %s · /sessions %d 查看 · %s", i+1, state.Status, i+1, state.Title), width))
		}
	}
	if len(notices) > limit {
		remaining := len(notices) - limit + 1
		notices = append(notices[:limit-1], displayLine(fmt.Sprintf("◆ 另有 %d 个后台会话有结果 · F2 查看", remaining), width))
	}
	return notices
}

// View 按标题、聊天正文、输入框顺序排版；留白来自区块间距，不填满终端高度。
func (m terminalModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	snapshot := m.session.snapshot()
	width := max(4, m.width-4) // 外层左右各留两列，不让正文紧贴终端边缘。

	currentNumber := 1
	var tabs []string
	for i, session := range m.app.opened {
		state := session.snapshot()
		label := fmt.Sprintf("%d %s", i+1, state.Status)
		if session == m.session {
			currentNumber = i + 1
			label = "[" + label + "]"
		} else if state.HasUnreadResult {
			label += " · 新结果"
		}
		tabs = append(tabs, label)
	}

	// 当前状态放在标题右侧，不再同时重复显示在标签栏和输入框上方。
	statusColor := lipgloss.Color("#929DAF")
	if snapshot.Running {
		statusColor = lipgloss.Color("#D9BC83")
	} else if snapshot.Status == "已完成" {
		statusColor = lipgloss.Color("#9BC5AE")
	} else if snapshot.Status == "失败" {
		statusColor = lipgloss.Color("#E3A0A0")
	}
	status := displayLine(snapshot.Status, width)
	title := displayLine(fmt.Sprintf("◆ 会话 %d · %s", currentNumber, snapshot.Title), max(1, width-ansi.StringWidth(status)-2))
	gap := max(1, width-ansi.StringWidth(title)-ansi.StringWidth(status))
	header := workspaceTitle.Render(title) + strings.Repeat(" ", gap) + lipgloss.NewStyle().Foreground(statusColor).Render(status)
	meta := m.session.config.Model + "  ·  " + m.session.config.ProjectDir
	header += "\n" + workspaceMuted.Render(displayLine(meta, width))
	if len(tabs) > 1 {
		header += "\n" + workspaceMuted.Render(displayLine(strings.Join(tabs, "    "), width))
	}

	// 输入框的边框、左右内边距各占两列。textarea 的宽度在 WindowSizeMsg 中同步调整。
	composer := composerPanel.Width(width - 2).Render(m.session.draft.View())
	help := "Enter 发送   Ctrl+J 换行   F2 会话   /help 更多"
	if width < 60 {
		help = "Enter 发送 · Ctrl+J 换行 · /help"
	}
	footer := workspaceMuted.Render(displayLine(help, width))
	var statusLines []string
	if snapshot.Running && snapshot.ProgressText != "" {
		statusLines = append(statusLines, workspaceMuted.Render(displayLine("◌ "+snapshot.ProgressText, width)))
	}
	for _, notice := range m.backgroundNotices(width, 2) {
		statusLines = append(statusLines, workspaceNotice.Render(notice))
	}
	statusBlock := ""
	if len(statusLines) > 0 {
		statusBlock = strings.Join(statusLines, "\n") + "\n\n"
	}

	// 用实际区块高度计算可见正文；没有足够消息时不补空行，输入框自然跟随正文。
	beforeBody := header + "\n\n"
	afterBody := "\n\n" + statusBlock + composer + "\n" + footer
	bodyHeight := max(1, m.height-2-strings.Count(beforeBody, "\n")-strings.Count(afterBody, "\n"))
	body := transcript(snapshot, width, bodyHeight, m.session.scrollOffset)
	beforeComposer := beforeBody + body + "\n\n" + statusBlock
	content := beforeComposer + composer + "\n" + footer
	view := tea.NewView(workspacePanel.Width(m.width).Render(content))

	// 中文输入法使用真实光标；偏移跟随区块实际行数，不依赖固定的屏幕底部坐标。
	view.Cursor = m.session.draft.Cursor()
	if view.Cursor != nil {
		view.Cursor.Position.X += 4                                       // 外层左留白 2 + 输入框边框 1 + 内边距 1。
		view.Cursor.Position.Y += strings.Count(beforeComposer, "\n") + 2 // 外层上留白 1 + 输入框上边框 1。
	}
	return view
}
