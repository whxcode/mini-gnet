package app

import (
	"fmt"
	"strings"
)

type commandName string

const (
	commandHelpName   commandName = "/help"
	commandConfigName commandName = "/config"
	commandResumeName commandName = "/resume"
	commandNewName    commandName = "/new"
	commandExitName   commandName = "/exit"
)

// commandHandler 直接持有当前 App 的操作权限，完成命令后只返回错误。
type commandHandler func(app *App, args []string) error

// command 把命令名称、说明、参数限制和实际处理函数放在同一条注册记录中。
type command struct {
	Name        commandName
	Description string
	MaxArgs     int
	Handler     commandHandler
}

// builtinCommands 是内置命令的唯一注册位置，同时决定帮助菜单的显示顺序。
func builtinCommands() []command {
	return []command{
		{Name: commandHelpName, Description: "查看内置命令和快捷键", Handler: handleHelp},
		{Name: commandConfigName, Description: "编辑配置，保存后立即生效", Handler: handleConfig},
		{Name: commandResumeName, Description: "选择并恢复历史会话", MaxArgs: 1, Handler: handleResume},
		{Name: commandNewName, Description: "开始新会话，保留当前记录", Handler: handleNew},
		{Name: commandExitName, Description: "退出程序", Handler: handleExit},
	}
}

// lookupCommand 只识别并查找命令，不执行 handler；普通问题返回 nil，错误命令返回 error。
func lookupCommand(query string) (*command, []string, error) {
	text := strings.TrimSpace(query)
	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, "\n\r") {
		return nil, nil, nil
	}
	fields := strings.Fields(text)
	name, args := fields[0], fields[1:]
	for _, cmd := range builtinCommands() {
		if cmd.Name != commandName(name) {
			continue
		}
		if len(args) > cmd.MaxArgs {
			return nil, nil, fmt.Errorf("%s 不接受这些参数", name)
		}
		return &cmd, args, nil
	}
	// 路径开头的提问仍交给模型，例如 /Users/mac/project 下有什么文件。
	if strings.Contains(strings.TrimPrefix(name, "/"), "/") {
		return nil, nil, nil
	}
	return nil, nil, fmt.Errorf("未知命令 %s，输入 %s 查看可用命令", name, commandHelpName)
}

// commandHelp 从同一注册列表生成帮助文本，不另外维护一份命令名称列表。
func commandHelp() string {
	var lines []string
	for _, cmd := range builtinCommands() {
		lines = append(lines, string(cmd.Name)+"  "+cmd.Description)
	}
	return strings.Join(lines, "\n") + "\n\nEnter 发送 · Shift+Enter / Ctrl+J 换行\n菜单：↑↓ 选择 · Enter 确认 · Esc 返回\n/config：Tab / ↑↓ 切换字段 · Esc 放弃修改\n/resume <会话ID> 也可直接恢复指定会话"
}
