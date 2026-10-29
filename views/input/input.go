// Package input 负责终端中的问题编辑，不负责调用模型或执行工具。
package input

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// Bubble Tea 的 Model 保存界面状态；Update 处理事件，View 负责显示。
// textarea 自己管理中文字符、光标、插入、删除及滚动，无需手工按字节操作字符串。
type editor struct {
	text      textarea.Model
	username  string
	submitted bool
	cancelled bool
	enhanced  bool
}

// NewTextArea 创建可复用的多行编辑框，各会话各持有一份，保留内容和光标。
func NewTextArea() textarea.Model {
	t := textarea.New()
	t.SetStyles(editorStyles())
	t.Placeholder = "输入消息…"
	t.ShowLineNumbers = false
	t.Prompt = "› "
	t.CharLimit = 0
	t.MaxWidth = 0
	// 由组件按实际显示行数计算高度：手动换行、长文本折行会撑开，删除后会收缩。
	// MaxHeight=0 不设固定高度上限，避免内容在一个三行窗口里滚动。
	t.DynamicHeight = true
	t.MinHeight = 1
	t.MaxHeight = 0
	t.SetWidth(80)
	t.SetVirtualCursor(false)
	t.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter", "ctrl+j"))
	t.Focus()
	return t
}

// newEditor 为单次输入包装编辑框，保留原有终端与管道接口。
func newEditor() editor {
	return editor{text: NewTextArea(), username: conversation.Username()}
}

// Init 单次编辑器不需要后台任务。
func (m editor) Init() tea.Cmd { return nil }

// Update 处理提交与退出，普通编辑和粘贴由 textarea 处理。
func (m editor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// 左右各留一列面板内边距，再留一列避免终端在最右侧自动折行。
		// SetWidth 会根据新的折行结果重新计算高度，这里不要再调用 SetHeight 固定高度。
		m.text.SetWidth(max(4, msg.Width-3))
	case tea.KeyboardEnhancementsMsg:
		m.enhanced = msg.SupportsKeyDisambiguation()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "enter", "ctrl+m":
			// 只在独立的 Enter 按键上提交。PasteMsg 中的换行交给 textarea 插入，
			// 防止粘贴多行代码时把每一行误当成一条提问。
			if strings.TrimSpace(m.text.Value()) != "" {
				m.submitted = true
				return m, tea.Quit
			}
			return m, nil
		}
	}

	// 其他事件（方向键、退格、Delete、粘贴、Shift+Enter 等）交给组件处理。
	var cmd tea.Cmd
	m.text, cmd = m.text.Update(msg)
	return m, cmd
}

// View 绘制单次编辑框及真实终端光标。
func (m editor) View() tea.View {
	if m.cancelled {
		return tea.NewView("")
	}
	if m.submitted {
		// 发送后由主循环统一打印带角色标题的用户消息，避免出现两份输入记录。
		return tea.NewView("")
	}
	help := "Enter 发送 · Shift+Enter 换行 · Ctrl+J 备用换行 · Ctrl+C 退出"
	if !m.enhanced {
		help += "\n若 Shift+Enter 无效，请用 Ctrl+J，或配置终端的 Shift+Enter 按键。"
	}
	header := ansi.Truncate(conversation.Header(conversation.User, m.username), m.text.Width(), "…")
	content := header + "\n" + m.text.View() + "\n" + inputHelp.Render(help)
	// Width 包含左右内边距，使标题、正文、提示和每行末尾都铺上连续背景。
	v := tea.NewView(inputPanel.Width(m.text.Width() + 2).Render(content))
	// 使用真实终端光标，使中文输入法候选窗尽量跟随编辑位置。
	v.Cursor = m.text.Cursor()
	if v.Cursor != nil {
		v.Cursor.Position.Y++ // 光标需要加上新增的用户标题这一行。
		v.Cursor.Position.X++ // 外层面板左侧还有一列内边距。
	}
	// Bubble Tea v2 默认协商按键区分协议；支持的终端会把 Shift+Enter
	// 上报为单独按键，不支持时它可能与 Enter 完全相同，不能靠 Go 猜测。
	return v
}
