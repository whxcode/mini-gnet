package input

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func send(m editor, msg tea.Msg) editor {
	next, _ := m.Update(msg)
	return next.(editor)
}

func TestEditorChineseEditing(t *testing.T) {
	m := send(newEditor(), tea.PasteMsg{Content: "你好世界"})
	m = send(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m = send(m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m = send(m, tea.KeyPressMsg{Code: tea.KeyDelete})
	m = send(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = send(m, tea.KeyPressMsg{Code: '中', Text: "中"})
	if got := m.text.Value(); got != "你中界" {
		t.Fatalf("在中文字符中间移动、删除、插入后得到 %q", got)
	}
}

func TestEditorNewlineAndSubmit(t *testing.T) {
	for _, newline := range []tea.KeyPressMsg{
		{Code: tea.KeyEnter, Mod: tea.ModShift},
		{Code: tea.KeyEnter, Mod: tea.ModAlt},
		{Code: 'j', Mod: tea.ModCtrl},
	} {
		t.Run(newline.String(), func(t *testing.T) {
			m := send(newEditor(), tea.PasteMsg{Content: "第一行"})
			m = send(m, newline)
			m = send(m, tea.PasteMsg{Content: "  第二行"})
			if got := m.text.Value(); m.submitted || got != "第一行\n  第二行" {
				t.Fatalf("换行不能发送或丢失缩进: %q, submitted=%v", got, m.submitted)
			}
			m = send(m, tea.KeyPressMsg{Code: tea.KeyEnter})
			if !m.submitted {
				t.Fatal("Enter 没有发送")
			}
		})
	}
}

func TestEditorPasteAndVerticalNavigation(t *testing.T) {
	m := send(newEditor(), tea.WindowSizeMsg{Width: 30, Height: 10})
	content := strings.Repeat("第一行\n", 30) + "最后一行"
	m = send(m, tea.PasteMsg{Content: content})
	if m.submitted || m.text.Value() != content {
		t.Fatal("长文本粘贴被截断或误触发发送")
	}
	m = send(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m = send(m, tea.KeyPressMsg{Code: tea.KeyHome})
	m = send(m, tea.KeyPressMsg{Code: 'X', Text: "X"})
	if !strings.HasSuffix(m.text.Value(), "X第一行\n最后一行") {
		t.Fatalf("向上移动并编辑失败: %q", m.text.Value())
	}
}

func TestEditorEmptyAndCancel(t *testing.T) {
	m := send(newEditor(), tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.submitted {
		t.Fatal("不应提交空问题")
	}
	m = send(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !m.cancelled || m.submitted {
		t.Fatal("Ctrl+C 应取消输入")
	}
}

// 除了直接发送按键事件，也测试终端字节序列经过 Bubble Tea 解码后的行为。
func TestTerminalKeySequences(t *testing.T) {
	for _, tc := range []struct{ name, wire, want string }{
		{"shift-enter", "你好\x1b[13;2u世界\r", "你好\n世界"},
		// Ctrl+J 在传统终端里是 LF 字节，不只是测试里手工构造的按键事件。
		{"ctrl-j-lf", "你好\x0a世界\r", "你好\n世界"},
		{"ctrl-j-kitty", "你好\x1b[106;5u世界\r", "你好\n世界"},
		{"ctrl-j-xterm", "你好\x1b[27;5;106~世界\r", "你好\n世界"},
		{"bracketed-paste", "\x1b[200~第一行\n  第二行\x1b[201~\r", "第一行\n  第二行"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := tea.NewProgram(newEditor(), tea.WithInput(strings.NewReader(tc.wire)),
				tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
			result, err := p.Run()
			if err != nil {
				t.Fatal(err)
			}
			m := result.(editor)
			if !m.submitted || m.text.Value() != tc.want {
				t.Fatalf("终端序列解码结果: %q, submitted=%v", m.text.Value(), m.submitted)
			}
		})
	}
}

func TestPipeInputKeepsBufferedLines(t *testing.T) {
	r := &Reader{lines: bufio.NewReader(strings.NewReader("第一条\n第二条"))}
	for _, want := range []string{"第一条\n", "第二条"} {
		got, err := r.Read()
		if err != nil || got != want {
			t.Fatalf("管道输入: got=%q, err=%v", got, err)
		}
	}
	if _, err := r.Read(); !errors.Is(err, io.EOF) {
		t.Fatalf("读取结束应返回 EOF，实际为 %v", err)
	}
}
