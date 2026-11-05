package app

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/views/contextusage"
)

// sessionStatus 统一标题和卡片上的图标、文字与颜色，不用颜色作为唯一状态提示。
func sessionStatus(state sessions.Snapshot) (string, color.Color) {
	if state.Status == "正在停止" {
		return "◌ 正在停止", lipgloss.Color("#D9BC83")
	}

	if state.Running || state.Status == "执行中" {
		return "◌ 执行中", lipgloss.Color("#D9BC83")
	}

	if len(state.Pending) > 0 {
		return "◷ 等待中", lipgloss.Color("#D9BC83")
	}

	switch state.Status {
	case "已完成":
		return "✓ 已完成", lipgloss.Color("#9BC5AE")
	case "失败":
		return "× 错误", lipgloss.Color("#E3A0A0")
	case "已停止":
		return "■ 已停止", lipgloss.Color("#929DAF")
	default:
		return "○ 空闲", lipgloss.Color("#929DAF")
	}
}

// sessionGrid 按窗口宽度排列卡片，编号始终对应 opened，切换或翻组都不会重新编号。
// 高度不足时显示快捷提示；会话较多时展示当前会话所在的一组，完整列表由 F2 打开。
func (m terminalModel) sessionGrid(width, maxHeight, currentNumber int) string {
	hint := "/ps 编号 切换 · F2 列表"

	if maxHeight < 7 {
		return workspaceMuted.Render(displayLine(hint, width))
	}

	// 一张卡片至少 24 列；两行统计加上标题和边框，每排六行，排间留一行。
	columns := min(4, max(1, (width+2)/26), len(m.app.opened))
	cardWidth := (width - (columns-1)*2) / columns
	rows := min(2, maxHeight/7)
	pageSize := columns * rows
	start := (currentNumber - 1) / pageSize * pageSize
	end := min(start+pageSize, len(m.app.opened))
	var gridRows []string

	for rowStart := start; rowStart < end; rowStart += columns {
		var cards []string

		for i := rowStart; i < min(rowStart+columns, end); i++ {
			session := m.app.opened[i]
			state := session.Snapshot()
			status, color := sessionStatus(state)
			label := fmt.Sprintf("%d  %s", i+1, status)
			borderColor := lipgloss.Color("#53617A")

			if session == m.session {
				label += " · 当前"
				borderColor = lipgloss.Color("#A7C7F5")
			}

			title := state.Title

			if len(state.Pending) > 0 {
				title = fmt.Sprintf("等待 %d 项 · %s", len(state.Pending), title)
			}

			if state.HasUnreadResult && session != m.session {
				title = "● 新结果 · " + title
			}

			// Lip Gloss v2 的 Width 包含边框与内边距，文字宽度需要扣除这四列。
			textWidth := max(1, cardWidth-4)
			content := lipgloss.NewStyle().Foreground(color).Render(displayLine(label, textWidth))
			content += "\n" + workspaceMuted.Render(displayLine(title, textWidth))
			content += "\n" + workspaceMuted.Render(displayLine(contextusage.UsedText(state.Usage), textWidth))
			content += "\n" + workspaceMuted.Render(displayLine(contextusage.RemainingText(state.Usage), textWidth))
			card := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(borderColor).
				Background(lipgloss.Color("#262B36")).
				Padding(0, 1).
				Width(cardWidth).
				Render(content)

			if len(cards) > 0 {
				cards = append(cards, "  ")
			}

			cards = append(cards, card)
		}

		gridRows = append(gridRows, lipgloss.JoinHorizontal(lipgloss.Top, cards...))
	}

	if end-start < len(m.app.opened) {
		hint = fmt.Sprintf("%d–%d / %d · %s", start+1, end, len(m.app.opened), hint)
	}

	return strings.Join(gridRows, "\n\n") + "\n" + workspaceMuted.Render(displayLine(hint, width))
}
