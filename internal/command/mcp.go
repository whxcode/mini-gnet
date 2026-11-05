package command

import (
	"fmt"

	"github.com/whxcode/mini-gnet/internal/mcp"
	menus "github.com/whxcode/mini-gnet/views/commands"
)

const mcpUsage = "用法：/mcp [add [名称 URL] | edit 名称 [URL] | show 名称 | search 关键词 | remove 名称]"

// handleMCP 分流命令参数或打开菜单，所有持久化由共享管理器完成。
func handleMCP(a Application, args []string) error {
	manager := a.MCPManager()
	if len(args) == 0 {
		return manageMCP(manager, "")
	}

	if args[0] == "add" && len(args) == 1 {
		return editMCP(manager, "")
	}

	if args[0] == "edit" && len(args) == 2 {
		return editMCP(manager, args[1])
	}

	if args[0] == "show" && len(args) == 2 {
		server, err := manager.Get(args[1])
		if err != nil {
			return err
		}

		return menus.ShowMCPDetail(server, manager.Path())
	}

	if args[0] == "search" && len(args) == 2 {
		return manageMCP(manager, args[1])
	}

	text, err := runMCPCommand(manager, args)
	if err != nil {
		return err
	}

	a.Notify(text)
	return nil
}

// manageMCP 按顺序展示列表、执行一次操作、刷新；界面不嵌套业务回调。
func manageMCP(manager *mcp.Manager, keyword string) error {
	var notice string
	for {
		action, err := menus.ManageMCP(manager.Search(keyword), manager.Path(), notice)
		if err != nil {
			return err
		}

		if action.Kind == "" {
			return nil
		}

		switch action.Kind {
		case "add":
			err = editMCP(manager, "")
		case "edit":
			err = editMCP(manager, action.Name)
		case "remove":
			err = manager.Remove(action.Name)
		}

		notice = ""
		if err != nil {
			notice = err.Error()
		}
	}
}

// editMCP 保留草稿直到保存成功或按 Esc；不会因磁盘写入失败丢弃用户输入。
func editMCP(manager *mcp.Manager, name string) error {
	server := mcp.Server{Transport: mcp.StreamableHTTP}
	if name != "" {
		var err error
		server, err = manager.Get(name)
		if err != nil {
			return err
		}
	}

	var problem string
	for {
		edited, err := menus.EditMCP(server, problem)
		if err != nil {
			return err
		}

		if edited == nil {
			return nil
		}

		if name == "" {
			err = manager.Add(*edited)
		} else {
			err = manager.Update(name, *edited)
		}

		if err == nil {
			return nil
		}

		server = *edited
		problem = err.Error()
	}
}

// runMCPCommand 执行不依赖终端的操作，供交互命令和管道共用。
func runMCPCommand(manager *mcp.Manager, args []string) (string, error) {
	if len(args) == 0 {
		return menus.MCPText(manager.List(), manager.Path()), nil
	}

	switch args[0] {
	case "add":
		if len(args) != 3 {
			return "", fmt.Errorf("%s；交互终端也可用 /mcp add 打开表单", mcpUsage)
		}

		server := mcp.Server{
			Name:      args[1],
			Transport: mcp.StreamableHTTP,
			URL:       args[2],
		}

		if err := manager.Add(server); err != nil {
			return "", err
		}

		return "已添加 MCP 服务 " + server.Name + " · 未连接", nil

	case "edit":
		if len(args) != 3 {
			return "", fmt.Errorf("%s；交互终端也可用 /mcp edit 名称 打开表单", mcpUsage)
		}

		server, err := manager.Get(args[1])
		if err != nil {
			return "", err
		}

		server.URL = args[2]
		if err := manager.Update(args[1], server); err != nil {
			return "", err
		}

		return "已更新 MCP 服务 " + server.Name + " · 未连接", nil

	case "show":
		if len(args) != 2 {
			return "", fmt.Errorf("%s", mcpUsage)
		}

		server, err := manager.Get(args[1])
		if err != nil {
			return "", err
		}

		return menus.MCPText([]mcp.Server{server}, manager.Path()), nil

	case "search":
		if len(args) != 2 {
			return "", fmt.Errorf("%s", mcpUsage)
		}

		return menus.MCPText(manager.Search(args[1]), manager.Path()), nil

	case "remove":
		if len(args) != 2 {
			return "", fmt.Errorf("%s", mcpUsage)
		}

		if err := manager.Remove(args[1]); err != nil {
			return "", err
		}

		return "已删除 MCP 服务 " + args[1], nil
	}

	return "", fmt.Errorf("%s", mcpUsage)
}
