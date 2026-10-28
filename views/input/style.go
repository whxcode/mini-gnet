package input

import (
	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
)

var (
	inputBackground = lipgloss.Color("#303544")
	inputForeground = lipgloss.Color("#E5E9F0")
	inputMuted      = lipgloss.Color("#A5ADBD")

	inputPanel = lipgloss.NewStyle().
			Background(inputBackground).
			Foreground(inputForeground).
			Padding(0, 1)
	inputHelp = lipgloss.NewStyle().
			Background(inputBackground).
			Foreground(inputMuted)
)

func editorStyles() textarea.Styles {
	styles := textarea.DefaultDarkStyles()
	// textarea 会分别渲染普通行、光标所在行与空白行。
	// 只设置外层背景不够：默认 CursorLine 的黑色背景会盖在上面，形成一条色带。
	// 所以这些状态统一使用面板背景，只有选中的文字保留额外高亮。
	for _, state := range []*textarea.StyleState{&styles.Focused, &styles.Blurred} {
		state.Base = lipgloss.NewStyle().Background(inputBackground).Foreground(inputForeground)
		state.Text = lipgloss.NewStyle().Background(inputBackground).Foreground(inputForeground)
		state.CursorLine = state.Text
		state.EndOfBuffer = state.Text
		state.Prompt = lipgloss.NewStyle().Background(inputBackground).Foreground(inputMuted)
		state.Placeholder = state.Prompt
		state.Selection = lipgloss.NewStyle().Background(lipgloss.Color("#505B73")).Foreground(inputForeground)
	}
	styles.Cursor.Color = inputForeground
	return styles
}
