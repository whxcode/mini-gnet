package command

import (
	"fmt"

	"github.com/whxcode/mini-gnet/tools"
	menus "github.com/whxcode/mini-gnet/views/commands"
)

// handleTools 展示只读工具说明；Esc 返回，不执行工具或写入模型历史。
func handleTools(a Application, args []string) error {
	items, err := selectTools(a.ToolInfos(), args)
	if err != nil {
		return err
	}

	return menus.ShowTools(items, len(args) > 0)
}

// selectTools 让交互菜单与管道使用相同的工具选择规则。
func selectTools(items []tools.Info, args []string) ([]tools.Info, error) {
	if len(args) == 0 {
		return items, nil
	}

	if len(args) != 1 {
		return nil, fmt.Errorf("用法：/tools 或 /tools <名称>")
	}

	for _, item := range items {
		if item.Definition.Name == args[0] {
			return []tools.Info{item}, nil
		}
	}

	return nil, fmt.Errorf("未注册工具 %s，输入 /tools 查看列表", args[0])
}

// toolsText 生成不带终端颜色和宽度限制的管道输出。
func toolsText(items []tools.Info, args []string) (string, error) {
	selected, err := selectTools(items, args)
	if err != nil {
		return "", err
	}

	return menus.ToolsText(selected, len(args) > 0), nil
}
