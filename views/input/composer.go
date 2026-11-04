package input

import (
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
	"github.com/whxcode/mini-gnet/internal/query"
)

// Composer 使用 Bubbles 处理文字编辑，用一个内部字符表示一个引用，让移动和删除天然保持原子性。
// 内部字符只存在于编辑器；显示、复制和提交前都转换回真实片段，不写进历史或模型请求。
type Composer struct {
	text       textarea.Model
	references map[rune]query.Segment
	nextRune   rune
	width      int
	MaxHeight  int // 只限制显示高度，不截断长输入。
}

// NewComposer 创建会话草稿；折行由标签布局负责，Bubbles 只维护逻辑行和编辑位置。
func NewComposer() Composer {
	text := NewTextArea()
	text.SetWidth(80)
	return Composer{
		text:       text,
		references: make(map[rune]query.Segment),
		nextRune:   0xF0000,
		width:      80,
	}
}

// Reset 清空草稿与引用映射，不影响已经复制给任务的输入。
func (m *Composer) Reset() {
	m.text.Reset()
	m.references = make(map[rune]query.Segment)
	m.nextRune = 0xF0000
}

// SetWidth 更新显示宽度，编辑位置始终以逻辑字符计数。
func (m *Composer) SetWidth(width int) {
	m.width = max(4, width)
	m.text.SetWidth(m.width)
}

// SetValue 接受外部文本，完整引用转成标签；未闭合引用留在草稿供用户继续编辑。
func (m *Composer) SetValue(value string) {
	m.Reset()
	m.insertText(value)
	m.normalize()
}

// Value 返回可复制的引用语法，供本地命令识别使用。
func (m Composer) Value() string {
	return m.Segments().Source()
}

// Segments 返回独立的结构化输入，连续普通文字合并为一个片段。
func (m Composer) Segments() query.Input {
	return m.decode(m.text.Value())
}

// Submission 检查用户尚未完成的手输引用，再交出独立片段；任务执行不重复解析输入语法。
func (m Composer) Submission() (query.Input, error) {
	if _, err := query.Parse(m.text.Value()); err != nil {
		return nil, err
	}
	input := m.Segments()
	if err := input.Validate(); err != nil {
		return nil, err
	}

	return input, nil
}

// decode 将内部字符还原为片段，调用方拿到的新切片不与编辑状态共享。
func (m Composer) decode(value string) query.Input {
	var parts query.Input
	var text strings.Builder
	for _, char := range value {
		part, exists := m.references[char]
		if !exists {
			text.WriteRune(char)
			continue
		}
		if part.Kind == query.Text {
			text.WriteString(part.Value)
			continue
		}

		if text.Len() > 0 {
			parts = append(parts, query.Segment{Kind: query.Text, Value: text.String()})
			text.Reset()
		}
		parts = append(parts, part)
	}
	if text.Len() > 0 {
		parts = append(parts, query.Segment{Kind: query.Text, Value: text.String()})
	}

	return parts
}

// encodeReference 为引用分配内部字符，所有输入中的私用字符也会单独编码，避免与引用碰撞。
func (m *Composer) encodeReference(part query.Segment) rune {
	char := m.nextRune
	m.nextRune++
	m.references[char] = part
	return char
}

// insertText 只插入文字，外部文本永远不能伪装成已有的内部引用。
func (m *Composer) insertText(value string) {
	var encoded strings.Builder
	for _, char := range value {
		if char >= 0xF0000 {
			char = m.encodeReference(query.Segment{Kind: query.Text, Value: string(char)})
		}
		encoded.WriteRune(char)
	}
	m.text.DeleteSelection()
	m.text.InsertString(encoded.String())
}

// InsertSkill 将候选技能作为整体插入光标处，不读取正文，也不设置会话级待选状态。
func (m *Composer) InsertSkill(name string) {
	m.text.DeleteSelection()
	m.text.InsertRune(m.encodeReference(query.Segment{Kind: query.Skill, Value: name}))
}

// offset 返回光标在内部缓冲区中的字符偏移，用于布局、候选替换和引用转换。
func (m Composer) offset() int {
	return logicalOffset(m.text.Value(), textarea.Position{Row: m.text.Line(), Col: m.text.Column()})
}

// logicalOffset 把 Bubbles 的逻辑行列转换为字符偏移，不按 UTF-8 字节计算中文位置。
func logicalOffset(value string, position textarea.Position) int {
	lines := strings.Split(value, "\n")
	offset := 0
	for row := 0; row < position.Row && row < len(lines); row++ {
		offset += utf8.RuneCountInString(lines[row]) + 1
	}

	return offset + position.Col
}

// moveTo 用 Bubbles 的左右键更新光标和选择，引用在底层恰好占一个字符。
func (m *Composer) moveTo(target int, selecting bool) {
	target = min(max(0, target), utf8.RuneCountInString(m.text.Value()))
	if !selecting {
		m.text.ClearSelection()
	}
	for m.offset() != target {
		key := tea.KeyPressMsg{Code: tea.KeyRight}
		if target < m.offset() {
			key.Code = tea.KeyLeft
		}
		if selecting {
			key.Mod = tea.ModShift
		}
		before := m.offset()
		m.text, _ = m.text.Update(key)
		if m.offset() == before {
			break
		}
	}
}

// normalize 只在用户输入或粘贴完整语法后转换标签，保留代码示例和当前光标位置。
func (m *Composer) normalize() {
	// 只解析尚未转换的原始文字，已有标签的内部字符保持不变。
	source := m.text.Value()
	if !strings.Contains(source, "/skills{") {
		return
	}
	parts, err := query.Parse(source)
	if err != nil {
		return
	}

	cursor := m.offset()
	consumedBytes := 0
	consumedRunes := 0
	target := 0
	outputLength := 0
	var encoded strings.Builder
	for _, part := range parts {
		value := part.Value
		sourceBytes := len(value)
		if part.Kind == query.Skill {
			sourceBytes = strings.IndexByte(source[consumedBytes:], '}') + 1
			value = string(m.encodeReference(part))
		}

		length := utf8.RuneCountInString(source[consumedBytes : consumedBytes+sourceBytes])
		if cursor > consumedRunes {
			target = outputLength + utf8.RuneCountInString(value)
			if part.Kind == query.Text {
				target = outputLength + min(length, cursor-consumedRunes)
			}
		}
		encoded.WriteString(value)
		outputLength += utf8.RuneCountInString(value)
		consumedRunes += length
		consumedBytes += sourceBytes
	}
	m.text.SetValue(encoded.String())
	m.moveTo(target, false)
}

// clipboardWriteMsg 只回传复制错误，不触碰草稿或选区。
type clipboardWriteMsg struct{ err error }

// clipboardTextMsg 保留读取失败，避免用空内容替换选区。
type clipboardTextMsg struct {
	text string
	err  error
}

// Update 保留 Bubbles 的编辑快捷键；标签布局的上下移动和剪贴板转换在这里处理。
func (m Composer) Update(msg tea.Msg) (Composer, tea.Cmd) {
	switch msg := msg.(type) {
	case clipboardWriteMsg:
		m.text.Err = msg.err
		return m, nil
	case clipboardTextMsg:
		m.text.Err = msg.err
		if msg.err == nil {
			m.insertText(msg.text)
			m.normalize()
		}
		return m, nil
	case tea.PasteMsg:
		m.insertText(msg.Content)
		m.normalize()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+v":
			return m, readClipboard
		case "ctrl+shift+c":
			if !m.text.HasSelection() {
				return m, nil
			}
			text := m.decode(m.text.SelectedText()).Source()
			return m, func() tea.Msg {
				return clipboardWriteMsg{err: clipboard.WriteAll(text)}
			}
		case "up", "ctrl+p", "shift+up", "down", "ctrl+n", "shift+down":
			m.moveVertical(msg)
			return m, nil
		}
		if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModMeta) == 0 {
			m.insertText(msg.Text)
			if strings.Contains(msg.Text, "}") {
				m.normalize()
			}
			return m, nil
		}
	}

	var command tea.Cmd
	m.text, command = m.text.Update(msg)
	return m, command
}

// readClipboard 读取系统剪贴板，仍交给 Composer 的文字入口处理引用和多行内容。
func readClipboard() tea.Msg {
	text, err := clipboard.ReadAll()
	return clipboardTextMsg{text: text, err: err}
}
