package commands

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/tools"
)

// toolSection 将标题与正文分开，使颜色、换行和留白由视图统一控制。
type toolSection struct {
	title string
	text  string
}

// toolCard 是一个工具的展示内容，不持有可执行入口。
type toolCard struct {
	name     string
	sections []toolSection
}

// toolCards 按名称、简介、输入参数、执行方式的顺序整理工具信息。
func toolCards(items []tools.Info, detail bool) []toolCard {
	var cards []toolCard

	for _, item := range items {
		definition := item.Definition
		launch := item.Launch
		card := toolCard{
			name: definition.Name,
			sections: []toolSection{
				{title: "简介", text: definition.Description},
				{title: "输入参数", text: parameterText(definition.InputSchema)},
			},
		}

		if launch.Command != "" {
			args := launch.Args
			if args == nil {
				args = []string{}
			}

			arguments, _ := json.Marshal(args)
			path := launch.ExecutablePath
			if path == "" {
				path = "当前环境未找到可执行文件"
			}

			card.sections = append(card.sections,
				toolSection{title: "执行命令", text: launchCommand(launch)},
				toolSection{title: "启动参数 · argv", text: string(arguments)},
				toolSection{title: "可执行文件路径", text: path},
				toolSection{title: "工作目录 · cwd", text: launch.WorkDir},
			)
		} else {
			card.sections = append(card.sections, toolSection{
				title: "执行方式",
				text:  "程序内调用；未提供进程启动信息。",
			})
		}

		if launch.ToolDir != "" {
			card.sections = append(card.sections, toolSection{title: "工具目录", text: launch.ToolDir})
		}

		if launch.ConfigPath != "" {
			card.sections = append(card.sections, toolSection{title: "配置文件", text: launch.ConfigPath})
		}

		if launch.Note != "" {
			card.sections = append(card.sections, toolSection{title: "调用说明", text: launch.Note})
		}

		if detail {
			data, _ := json.MarshalIndent(definition, "", "  ")
			card.sections = append(card.sections, toolSection{title: "完整定义 · Schema", text: string(data)})
		}

		cards = append(cards, card)
	}

	return cards
}

// launchCommand 生成便于阅读的命令预览；实际执行仍使用独立的 command 和 args。
func launchCommand(launch tools.LaunchInfo) string {
	parts := append([]string{launch.Command}, launch.Args...)
	for i, part := range parts {
		if part == "" || strings.ContainsAny(part, " \t\n\"'\\;$&|<>()`") {
			parts[i] = strconv.Quote(part)
		}
	}

	return strings.Join(parts, " ")
}

// parameterText 将每个参数的名称、类型与说明分行展示，必填项排在前面。
func parameterText(schema json.RawMessage) string {
	var input struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}

	if err := json.Unmarshal(schema, &input); err != nil {
		return string(schema)
	}

	var names []string
	for name := range input.Properties {
		names = append(names, name)
	}

	sort.Strings(names)
	sort.SliceStable(names, func(i, j int) bool {
		return slices.Contains(input.Required, names[i]) && !slices.Contains(input.Required, names[j])
	})

	var parameters []string
	for _, name := range names {
		property := input.Properties[name]
		required := "可选"
		if slices.Contains(input.Required, name) {
			required = "必填"
		}

		text := fmt.Sprintf("%s (%s，%s)", name, property.Type, required)
		if property.Description != "" {
			text += "\n  " + property.Description
		}

		parameters = append(parameters, text)
	}

	if len(parameters) == 0 {
		return "无输入参数"
	}

	return strings.Join(parameters, "\n\n")
}

// ToolsText 为管道保留完整内容和路径，不插入 ANSI 颜色或按屏幕宽度折行。
func ToolsText(items []tools.Info, detail bool) string {
	if len(items) == 0 {
		return "当前没有已注册工具。"
	}

	var text strings.Builder
	fmt.Fprintf(&text, "当前已注册 %d 个工具\n", len(items))

	for _, card := range toolCards(items, detail) {
		fmt.Fprintf(&text, "\n── %s ──\n", card.name)
		for _, section := range card.sections {
			fmt.Fprintf(&text, "\n%s\n%s\n", section.title, section.text)
		}
	}

	return text.String()
}

// render 绘制有边框的工具卡片；正文按显示宽度换行，路径不会被省略。
func (c toolCard) render(width int) string {
	contentWidth := max(1, width-4)
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#8BD5CA")).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#91B9F5"))
	bodyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E9F0"))

	lines := []string{titleStyle.Render(ansi.Hardwrap(cleanToolText(c.name), contentWidth, true))}
	for _, section := range c.sections {
		lines = append(lines,
			"",
			labelStyle.Render(ansi.Hardwrap(section.title, contentWidth, true)),
			bodyStyle.Render(ansi.Hardwrap(cleanToolText(section.text), contentWidth, true)),
		)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#596580")).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(lines, "\n"))
}

// cleanToolText 防止用户工具描述中的终端控制字符影响布局，保留正文换行。
func cleanToolText(text string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || !unicode.IsControl(r) {
			return r
		}

		return ' '
	}, ansi.Strip(text))
}
