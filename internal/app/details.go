package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// setScrollOffset 更新当前视图的滚动位置，详情和聊天互不覆盖。
func (m *terminalModel) setScrollOffset(offset int) {
	if m.details {
		m.detailOffset = offset

		return
	}

	m.session.ScrollOffset = offset
}

// renderDetails 在同一个终端程序内展示完整记录；后台 Session.Run 继续执行。
func (m terminalModel) renderDetails() (tea.View, conversation.Transcript) {
	snapshot := m.session.Snapshot()
	width := max(4, m.width-4)
	height := max(1, m.height-6)
	body := conversation.NewTranscript(snapshot.Messages, snapshot.StreamText, width, height, m.detailOffset)

	if m.detailOffset > 0 && m.lastLines > 0 && body.Total > m.lastLines {
		offset := m.detailOffset + body.Total - m.lastLines
		body = conversation.NewTranscript(snapshot.Messages, snapshot.StreamText, width, height, offset)
	}

	header := workspaceTitle.Render(displayLine("完整执行记录 · 当前会话", width))
	footer := workspaceMuted.Render(displayLine("Esc 返回 · 滚轮/PgUp/PgDn · Ctrl+Home 顶部 · Ctrl+End 最新", width))
	view := tea.NewView(workspacePanel.Width(m.width).Render(header + "\n\n" + body.Content + "\n\n" + footer))
	view.MouseMode = tea.MouseModeCellMotion

	return view, body
}
