package command

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/whxcode/mini-gnet/tools"
	menus "github.com/whxcode/mini-gnet/views/commands"
)

// handleTools 展示只读工具说明；Esc 返回，不执行工具或写入模型历史。
func handleTools(a Application, args []string) error {
	text, err := toolsText(a.ToolDefinitions(), args)
	if err != nil {
		return err
	}

	return menus.ShowTools(text)
}

// toolsText 生成交互和管道共用的说明，列表列参数，指定名称时显示完整 schema。
func toolsText(definitions []tools.Definition, args []string) (string, error) {
	if len(args) > 0 {
		for _, definition := range definitions {
			if definition.Name != args[0] {
				continue
			}

			data, err := json.MarshalIndent(definition, "", "  ")

			return string(data), err
		}

		return "", fmt.Errorf("未注册工具 %s，输入 /tools 查看列表", args[0])
	}

	if len(definitions) == 0 {
		return "当前没有已注册工具。", nil
	}

	var text strings.Builder
	fmt.Fprintf(&text, "当前已注册 %d 个工具\n", len(definitions))

	for _, definition := range definitions {
		fmt.Fprintf(&text, "\n%s\n%s\n参数：\n", definition.Name, definition.Description)
		var input struct {
			Properties map[string]struct {
				Type        string `json:"type"`
				Description string `json:"description"`
			} `json:"properties"`
			Required []string `json:"required"`
		}

		if err := json.Unmarshal(definition.InputSchema, &input); err != nil {
			return "", err
		}

		var names []string

		for name := range input.Properties {
			names = append(names, name)
		}

		sort.Strings(names)

		for _, name := range names {
			property := input.Properties[name]
			required := "可选"

			if slices.Contains(input.Required, name) {
				required = "必填"
			}

			fmt.Fprintf(&text, "  %s (%s，%s)：%s\n", name, property.Type, required, property.Description)
		}
	}

	text.WriteString("\n统一结果：ok / data / error；/tools <名称> 查看输入和输出的完整定义。")

	return text.String(), nil
}
