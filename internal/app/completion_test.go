package app

import (
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestInlineSkillCompletion 验证候选确认与发送分离，标签保留后文，Esc 不丢草稿。
func TestInlineSkillCompletion(t *testing.T) {
	a := skillTestApp(t, "")
	model := newTerminal(a, context.Background())
	model = updateTerminal(model, tea.PasteMsg{Content: "请使用 /sk"})
	if len(model.completions) != 1 || !model.completions[0].skill {
		t.Fatal("句子中应该只显示匹配技能")
	}
	for _, width := range []int{24, 40, 80} {
		model = updateTerminal(model, tea.WindowSizeMsg{Width: width, Height: 24})
		view := model.View()
		plain := ansi.Strip(view.Content)
		lines := strings.Split(plain, "\n")
		if !strings.Contains(plain, "Esc") || len(lines) > 24 || view.Cursor == nil || view.Cursor.Position.Y >= len(lines) {
			t.Fatalf("候选高度或光标错误：\n%s", plain)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("%d 列候选溢出：%q", width, line)
			}
		}
	}

	model = updateTerminal(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.done || len(model.completions) != 0 || a.CurrentSession.Draft.Value() != "请使用 /skills{code-review}" {
		t.Fatal("确认候选不应发送任务")
	}
	model = updateTerminal(model, tea.PasteMsg{Content: " 检查项目"})
	model = updateTerminal(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !model.done || !model.fromEditor || model.query != "请使用 /skills{code-review} 检查项目" {
		t.Fatal("第二次 Enter 没有提交完整问题")
	}

	a.CurrentSession.Draft.SetValue("继续 /sk")
	model = newTerminal(a, context.Background())
	model.refreshCompletions()
	model = updateTerminal(model, tea.KeyPressMsg{Code: tea.KeyEscape})
	if model.done || len(model.completions) != 0 || a.CurrentSession.Draft.Value() != "继续 /sk" {
		t.Fatal("Esc 不应发送、删除或转换草稿")
	}
}

// TestCommandCompletionAndPaths 验证句首命令与技能分组，路径和代码中的斜线不触发候选。
func TestCommandCompletionAndPaths(t *testing.T) {
	a := skillTestApp(t, "")
	model := newTerminal(a, context.Background())
	model = updateTerminal(model, tea.PasteMsg{Content: "/"})
	hasCommand := false
	hasSkill := false
	for _, item := range model.completions {
		hasSkill = hasSkill || item.skill
		hasCommand = hasCommand || !item.skill
	}
	if !hasCommand || !hasSkill {
		t.Fatal("句首 / 应提供命令与技能候选")
	}
	for _, text := range []string{"/tmp/code", "https://host/path", "```go\n/sk"} {
		a.CurrentSession.Draft.SetValue(text)
		model.refreshCompletions()
		if len(model.completions) != 0 {
			t.Fatalf("不应补全路径或代码：%q", text)
		}
	}
}

// TestSkillInputTerminalSequence 验证真实终端字节解码后，候选确认、多行输入和发送能连贯完成。
func TestSkillInputTerminalSequence(t *testing.T) {
	a := skillTestApp(t, "")
	wire := "\x1b[200~请使用 /sk\x1b[201~\r\x1b[200~ 检查项目\x1b[201~\x0a第二行\r"
	program := tea.NewProgram(newTerminal(a, context.Background()),
		tea.WithInput(strings.NewReader(wire)),
		tea.WithOutput(io.Discard),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
	)
	result, err := program.Run()
	if err != nil {
		t.Fatal(err)
	}
	model := result.(terminalModel)
	if !model.done || !model.fromEditor || model.query != "请使用 /skills{code-review} 检查项目\n第二行" {
		t.Fatalf("终端输入未按预期提交：%q", model.query)
	}
}
