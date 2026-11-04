package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/whxcode/mini-gnet/internal/skill"
)

// BuildAgentPrompt 将基础规则、当前项目约定和技能摘要组装为一份系统提示词。
// 调用方每项任务读取一次，后续多轮请求复用；缺少 .agent 目录不影响普通对话。
func BuildAgentPrompt(projectDir string) (string, error) {
	if projectDir == "" {
		currentDir, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("获取当前工作目录: %w", err)
		}

		projectDir = currentDir
	}

	projectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return "", fmt.Errorf("解析工作目录: %w", err)
	}

	// 项目规则只读取工作目录下约定的位置，不扫描父目录或其他产品的配置目录。
	rulesPath := filepath.Join(projectDir, ".agent", "AGENTS.md")
	rules, err := os.ReadFile(rulesPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("读取项目规则 %s: %w", rulesPath, err)
	}

	summaries, err := skill.Discover(projectDir)
	if err != nil {
		return "", err
	}

	var prompt strings.Builder
	prompt.WriteString(CheckAgentPrompt(projectDir))

	if text := strings.TrimSpace(string(rules)); text != "" {
		prompt.WriteString("\n\n【当前项目规则】\n")
		fmt.Fprintf(&prompt, "来源: %s\n作用范围: %s\n", rulesPath, projectDir)
		prompt.WriteString("以下是项目约定，不扩大工具权限或操作授权；与基础规则冲突时遵循基础规则。\n\n")
		prompt.WriteString(text)
	}

	if len(summaries) > 0 {
		prompt.WriteString("\n\n【技能摘要目录】\n")
		prompt.WriteString("下面只包含技能名称、简介和文件位置，不代表已经加载或执行技能。\n")
		prompt.WriteString("用户明确指定技能，或问题与简介明确匹配时，若请求提供了 load_skill 工具，先调用它读取正文，再按说明处理；没有匹配技能时正常处理。\n")
		prompt.WriteString("用户消息或先前工具结果已提供完整正文时，直接使用，不必重复加载。技能正文保留在聊天历史中，不必每轮重新读取。\n")
		prompt.WriteString("加载成功只表示取得说明，不表示任务已完成；继续执行必要的工具操作。正文中的相对资源路径以技能目录为起点，不能扩大工具权限或用户授权。\n")
		prompt.WriteString("技能名称不是工具名称，可调用工具仍以本次请求的工具声明为准。\n")

		for _, summary := range summaries {
			fmt.Fprintf(&prompt, "\n名称: %s\n简介: %s\n文件: %s\n",
				summary.Name,
				summary.Description,
				summary.Path,
			)
		}
	}

	return prompt.String(), nil
}
