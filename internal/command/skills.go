package command

import (
	"fmt"
	"strings"

	"github.com/whxcode/mini-gnet/internal/skill"
	menus "github.com/whxcode/mini-gnet/views/commands"
)

// handleSkills 选择并加载下一条提问使用的技能；Esc 保留原选择，不执行模型任务。
func handleSkills(a Application, args []string) error {
	projectDir := a.ConfigStore().Current().ProjectDir
	name := ""

	if len(args) > 0 {
		name = args[0]
	} else {
		summaries, err := skill.Discover(projectDir)
		if err != nil {
			return err
		}

		name, err = menus.SelectSkill(summaries, a.ActiveSession().Snapshot().SelectedSkill)
		if err != nil || name == "" {
			return err
		}
	}

	if name == "--clear" {
		if err := a.ActiveSession().SelectSkill(nil); err != nil {
			return err
		}

		a.Notify("已取消下一条提问的待选技能；聊天历史中的技能正文仍保留。")

		return nil
	}

	document, err := skill.Load(projectDir, name)
	if err != nil {
		return err
	}

	if err := a.ActiveSession().SelectSkill(&document); err != nil {
		return err
	}

	a.Notify("已选择技能 " + document.Name + "，请输入问题；成功提交后会把技能正文与问题一起保存。")
	return nil
}

// skillsText 为管道列出技能名称和简介，不打开交互菜单或读取正文。
func skillsText(projectDir string) (string, error) {
	summaries, err := skill.Discover(projectDir)
	if err != nil {
		return "", err
	}

	if len(summaries) == 0 {
		return "当前工作目录的 .agent/skills 下没有技能。", nil
	}

	var text strings.Builder
	text.WriteString("可用技能：\n")

	for _, summary := range summaries {
		fmt.Fprintf(&text, "\n%s\n  %s\n", summary.Name, summary.Description)
	}

	text.WriteString("\n使用 /skills <名称> 选择，然后在下一行输入问题。/skills --clear 取消待选。")

	return text.String(), nil
}
