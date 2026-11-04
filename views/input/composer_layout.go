package input

import (
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/internal/query"
)

// cellPosition 对应内部字符边界在屏幕上的位置，标签只有前后两个可落点。
type cellPosition struct {
	x int
	y int
}

// composerLayout 同时提供显示行和光标映射，避免两套折行算法导致光标偏移。
type composerLayout struct {
	lines     []string
	positions []cellPosition
}

// layout 将文字与标签折行；超宽标签只截断显示，真实名称与提交内容不受影响。
func (m Composer) layout() composerLayout {
	width := max(1, m.width-3) // 两列提示符，留一列放行尾光标。
	buffer := []rune(m.text.Value())
	result := composerLayout{positions: make([]cellPosition, len(buffer)+1)}
	line := ""
	point := cellPosition{}
	textStyle := lipgloss.NewStyle().Foreground(inputForeground).Background(inputBackground)
	skillStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5D3FF")).Background(lipgloss.Color("#504366")).Bold(true)
	selectionStyle := lipgloss.NewStyle().Foreground(inputForeground).Background(lipgloss.Color("#61728E"))
	start, end, selected := m.text.Selection()
	selectionStart := logicalOffset(m.text.Value(), start)
	selectionEnd := logicalOffset(m.text.Value(), end)

	for index, char := range buffer {
		value := string(char)
		style := textStyle
		if part, exists := m.references[char]; exists {
			value = part.Value
			if part.Kind == query.Skill {
				value = "◆ " + cleanLabel(value)
				style = skillStyle
			}
		}
		if value == "\n" {
			result.positions[index] = point
			result.lines = append(result.lines, line)
			line = ""
			point = cellPosition{y: point.y + 1}
			continue
		}

		value = ansi.Truncate(value, width, "…")
		length := ansi.StringWidth(value)
		if point.x+length > width {
			result.lines = append(result.lines, line)
			line = ""
			point = cellPosition{y: point.y + 1}
		}
		result.positions[index] = point
		if selected && index >= selectionStart && index < selectionEnd {
			style = selectionStyle
		}
		line += style.Render(value)
		point.x += length
	}
	result.positions[len(buffer)] = point
	result.lines = append(result.lines, line)
	return result
}

// cleanLabel 清除引用名称里的终端控制字符，渲染不改变原始标识。
func cleanLabel(value string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsControl(char) {
			return ' '
		}
		return char
	}, ansi.Strip(value))
}

// visibleStart 让编辑光标始终留在受高度限制的输入区中。
func (m Composer) visibleStart(layout composerLayout) int {
	if m.MaxHeight <= 0 {
		return 0
	}

	return max(0, layout.positions[m.offset()].y-m.MaxHeight+1)
}

// View 绘制真正的技能标签，底层占位字符与 /skills{...} 语法不会出现在界面里。
func (m Composer) View() string {
	layout := m.layout()
	start := m.visibleStart(layout)
	end := len(layout.lines)
	if m.MaxHeight > 0 {
		end = min(end, start+m.MaxHeight)
	}

	var lines []string
	for index := start; index < end; index++ {
		prefix := "  "
		if index == 0 {
			prefix = "› "
		}
		line := layout.lines[index]
		if m.text.Value() == "" {
			line = inputHelp.Render(ansi.Truncate("输入消息…", max(1, m.width-3), "…"))
		}
		lines = append(lines, inputHelp.Render(prefix)+line)
	}

	return strings.Join(lines, "\n")
}

// Cursor 使用标签布局中的真实坐标，中文输入法候选框也跟随该位置。
func (m Composer) Cursor() *tea.Cursor {
	layout := m.layout()
	position := layout.positions[m.offset()]
	cursor := tea.NewCursor(position.x+2, position.y-m.visibleStart(layout))
	cursor.Color = inputForeground
	return cursor
}

// moveVertical 按可见行寻找最近的字符边界，不能把光标放进技能标签内部。
func (m *Composer) moveVertical(key tea.KeyPressMsg) {
	layout := m.layout()
	position := layout.positions[m.offset()]
	targetRow := position.y + 1
	if key.Code == tea.KeyUp || key.String() == "ctrl+p" {
		targetRow = position.y - 1
	}
	if targetRow < 0 || targetRow >= len(layout.lines) {
		return
	}

	target := m.offset()
	bestDistance := m.width + 1
	for index, point := range layout.positions {
		if point.y != targetRow {
			continue
		}
		distance := point.x - position.x
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance {
			target = index
			bestDistance = distance
		}
	}
	m.moveTo(target, key.Mod&tea.ModShift != 0)
}
