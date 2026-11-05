package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/whxcode/mini-gnet/internal/skill"
)

// handleSkills 仅展示技能目录或指定技能说明，不修改草稿或设置下一条任务的技能。
func handleSkills(_ context.Context, a Application, args []string) error {
	text, err := skillsText(a.ConfigStore().Current().ProjectDir, args)
	if err != nil {
		return err
	}
	a.Notify(text)
	return nil
}

// skillsText 为交互与管道生成相同说明，用户直接在提问中引用技能。
func skillsText(projectDir string, args []string) (string, error) {
	if len(args) > 0 && args[0] == "--clear" {
		return "", fmt.Errorf("已改为行内技能标签；请直接在输入框删除标签，不再设置待选技能")
	}

	summaries, err := skill.Discover(projectDir)
	if err != nil {
		return "", err
	}
	if len(summaries) == 0 {
		return "当前工作目录的 .agent/skills 下没有技能。", nil
	}

	var text strings.Builder
	text.WriteString("可用技能：\n")
	found := len(args) == 0
	for _, summary := range summaries {
		if len(args) > 0 && summary.Name != args[0] {
			continue
		}
		found = true
		fmt.Fprintf(&text, "\n◆ %s\n  %s\n  /skills{%s}\n", summary.Name, summary.Description, summary.Name)
	}
	if !found {
		return "", fmt.Errorf("未找到技能 %q", args[0])
	}
	text.WriteString("\n在问题中输入 / 打开候选，或写：请使用 /skills{名称} 检查当前项目。")
	return text.String(), nil
}
