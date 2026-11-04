package input

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/internal/query"
)

// TestComposerAtomicSkill 验证显示标签、片段快照和整体移动删除，同时保留左右中文文字。
func TestComposerAtomicSkill(t *testing.T) {
	m := NewComposer()
	m.SetValue("请使用 /skills{review}检查")
	parts := m.Segments()
	want := query.Input{
		{Kind: query.Text, Value: "请使用 "},
		{Kind: query.Skill, Value: "review"},
		{Kind: query.Text, Value: "检查"},
	}
	if !reflect.DeepEqual(parts, want) || strings.Contains(m.View(), "/skills{") || !strings.Contains(ansi.Strip(m.View()), "◆ review") {
		t.Fatalf("标签未转换或显示错误：%+v / %s", parts, m.View())
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	before := m.Cursor().Position.X
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if before-m.Cursor().Position.X != ansi.StringWidth("◆ review") {
		t.Fatal("左键没有一次跨过完整标签")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
	if m.Value() != "请使用 检查" || !reflect.DeepEqual(parts, want) {
		t.Fatal("Delete 没有整块删除或快照被后续编辑修改")
	}

	m.SetValue("/skills{review}")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if m.Value() != "" {
		t.Fatal("Backspace 没有整块删除")
	}
}

// TestComposerSelectionAndPaste 验证多行粘贴、选区替换及私用字符不会误指向已有标签。
func TestComposerSelectionAndPaste(t *testing.T) {
	m := NewComposer()
	m.SetValue("/skills{review}")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	m, _ = m.Update(tea.PasteMsg{Content: "你好\n第二行"})
	if m.Value() != "你好\n第二行" {
		t.Fatal("选区替换残留标签", m.Value())
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m, _ = m.Update(tea.KeyPressMsg{Code: '中', Text: "中"})
	if m.Value() != "中你好\n第二行" {
		t.Fatal("中文上下移动编辑错误", m.Value())
	}

	m.SetValue("/skills{review}")
	m, _ = m.Update(tea.PasteMsg{Content: "\U000F0000"})
	if m.Value() != "/skills{review}\U000F0000" || len(m.Segments()) != 2 {
		t.Fatal("外部字符伪装成了技能标签")
	}
	m.SetValue("示例：`/skills{review}`")
	if len(m.Segments()) != 1 || m.Segments()[0].Kind != query.Text {
		t.Fatal("代码示例不应变成标签")
	}
}

// TestComposerCompletion 验证中途插入标签、保留后文以及代码和路径不触发候选。
func TestComposerCompletion(t *testing.T) {
	m := NewComposer()
	m.SetValue("请使用 /sk 后文")
	for range 3 {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	}
	if prefix, ok := m.CompletionPrefix(); !ok || prefix != "/sk" {
		t.Fatalf("没有识别光标前的候选词：%q", prefix)
	}
	m.Complete("review", true)
	if m.Value() != "请使用 /skills{review} 后文" {
		t.Fatal("替换候选破坏了后文", m.Value())
	}
	for _, text := range []string{"https://host/path", "/tmp/foo", "`/sk", "```\n/sk"} {
		m.SetValue(text)
		if prefix, active := m.CompletionPrefix(); active {
			t.Fatalf("%q 不应打开候选：%q", text, prefix)
		}
	}
}

// TestComposerWrapping 验证窄窗口、长标签、多行高度限制和光标使用同一套布局。
func TestComposerWrapping(t *testing.T) {
	for _, width := range []int{8, 20, 40} {
		m := NewComposer()
		m.SetWidth(width)
		m.MaxHeight = 3
		m.SetValue(strings.Repeat("中文内容", 10) + " /skills{very-long-skill-name} 尾")
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		cursor := m.Cursor().Position
		if len(lines) > 3 || cursor.Y >= len(lines) || cursor.X >= width || !strings.Contains(lines[cursor.Y], "尾") {
			t.Fatalf("%d 列的光标或高度错误：%+v / %q", width, cursor, lines)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("%d 列内容溢出：%q", width, line)
			}
		}
	}
}

// TestComposerPreservesExistingReferences 验证已有标签不因邻接文字或粘贴新引用变回普通文本。
func TestComposerPreservesExistingReferences(t *testing.T) {
	m := NewComposer()
	m.InsertSkill("review")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m, _ = m.Update(tea.PasteMsg{Content: "a"})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m, _ = m.Update(tea.PasteMsg{Content: " /skills{ second }"})
	parts, err := m.Submission()
	if err != nil || len(parts) != 4 || parts[1].Kind != query.Skill || parts[1].Value != "review" || parts[3].Value != "second" {
		t.Fatalf("已有引用被重新解释或新引用解析错误：%+v, %v", parts, err)
	}
	m.SetValue("请使用 /skills{review")
	if _, err := m.Submission(); err == nil || !strings.HasSuffix(m.Value(), "{review") {
		t.Fatal("不完整引用应该提示错误并保留原输入")
	}
}
