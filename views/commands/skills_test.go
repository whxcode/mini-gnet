package commands

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/internal/skill"
)

// TestSkillsSelection 验证默认选择、切换、清除和 Esc，不在菜单内执行技能。
func TestSkillsSelection(t *testing.T) {
	summaries := []skill.Summary{{Name: "first"}, {Name: "second"}}
	model := newSkills(summaries, "second")
	if model.cursor != 2 {
		t.Fatal("没有定位到当前待选技能")
	}

	next, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := next.(skillsModel); !got.done || got.selected != "" {
		t.Fatal("Esc 应保留空结果，不修改待选")
	}

	next, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if next.(skillsModel).selected != "first" {
		t.Fatal("上下键和 Enter 未选中技能")
	}

	for _, summaries := range [][]skill.Summary{nil, summaries} {
		model := newSkills(summaries, "")
		next, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyHome})
		next, _ = next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if next.(skillsModel).selected != "--clear" {
			t.Fatal("取消待选项没有返回明确操作")
		}
	}
}

// TestSkillsPagination 验证中文简介、窄窗口和大量技能时仍能看到最后一项和 Esc 提示。
func TestSkillsPagination(t *testing.T) {
	var summaries []skill.Summary
	for i := 0; i < 30; i++ {
		summaries = append(summaries, skill.Summary{
			Name:        fmt.Sprintf("skill-%02d", i),
			Description: strings.Repeat("检查中文长简介是否溢出", 10),
		})
	}

	for _, width := range []int{16, 32, 80} {
		model, _ := newSkills(summaries, "").Update(tea.WindowSizeMsg{Width: width, Height: 12})
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		plain := ansi.Strip(model.View().Content)
		if !strings.Contains(plain, "skill-29") || !strings.Contains(plain, "Esc") {
			t.Fatalf("最后一项或返回提示不可见：\n%s", plain)
		}
		lines := strings.Split(plain, "\n")
		if len(lines) > 12 {
			t.Fatal("菜单超过终端高度")
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > width {
				t.Fatalf("菜单超过 %d 列：%q", width, line)
			}
		}
	}
}
