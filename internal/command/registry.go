// Package command 负责本地命令的注册、解析与执行，不依赖 App 的具体实现。
package command

import (
	"fmt"
	"strings"
)

// Name 是本地命令名称，避免调用方散落命令字符串。
type Name string

const (
	HelpName     Name = "/help"
	ConfigName   Name = "/config"
	ResumeName   Name = "/resume"
	NewName      Name = "/new"
	ExitName     Name = "/exit"
	SessionsName Name = "/sessions"
	PSName       Name = "/ps"
	StopName     Name = "/stop"
	ToolsName    Name = "/tools"
)

// Handler 接收当前应用的操作接口，完成命令后只返回错误。
type Handler func(app Application, args []string) error

// Command 把命令名称、说明、参数限制和实际处理函数放在同一条注册记录中。
type Command struct {
	Name        Name    // 输入时使用的命令名称。
	Description string  // 帮助菜单中的说明。
	MaxArgs     int     // 最多接受的参数个数；0 表示无参数。
	Handler     Handler // 执行操作并返回错误。
}

// builtinCommands 是内置命令的唯一注册位置，同时决定帮助菜单的显示顺序。
func builtinCommands() []Command {
	return []Command{
		{
			Name:        HelpName,
			Description: "查看内置命令和快捷键",
			Handler:     handleHelp,
		},
		{
			Name:        ToolsName,
			Description: "查看工具和参数；/tools <名称> 查看完整定义",
			MaxArgs:     1,
			Handler:     handleTools,
		},
		{
			Name:        ConfigName,
			Description: "编辑配置，保存后立即生效",
			Handler:     handleConfig,
		},
		{
			Name:        ResumeName,
			Description: "选择并恢复历史会话",
			MaxArgs:     1,
			Handler:     handleResume,
		},
		{
			Name:        NewName,
			Description: "创建并切换新会话，旧任务继续执行",
			Handler:     handleNew,
		},
		{
			Name:        SessionsName,
			Description: "切换已打开的会话（可带编号或 ID）",
			MaxArgs:     1,
			Handler:     handleSessions,
		},
		{
			Name:        PSName,
			Description: "快速切换会话，例如 /ps 2；不带参数打开列表",
			MaxArgs:     1,
			Handler:     handleSessions,
		},
		{
			Name:        StopName,
			Description: "停止当前会话的任务并清空等待队列",
			Handler:     handleStop,
		},
		{
			Name:        ExitName,
			Description: "退出程序",
			Handler:     handleExit,
		},
	}
}

// Lookup 只识别并查找命令，不执行 handler；普通问题返回 nil，错误命令返回 error。
func Lookup(query string) (*Command, []string, error) {
	text := strings.TrimSpace(query)

	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, "\n\r") {
		return nil, nil, nil
	}

	fields := strings.Fields(text)
	name := fields[0]
	args := fields[1:]

	for _, cmd := range builtinCommands() {
		if cmd.Name != Name(name) {
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

	return nil, nil, fmt.Errorf("未知命令 %s，输入 %s 查看可用命令", name, HelpName)
}

// HelpText 从同一注册列表生成帮助文本，不另外维护一份命令名称列表。
func HelpText() string {
	var lines []string

	for _, cmd := range builtinCommands() {
		lines = append(lines, string(cmd.Name)+"  "+cmd.Description)
	}

	return strings.Join(lines, "\n") + "\n\n" +
		"Enter 发送 · Shift+Enter / Ctrl+J 换行\n" +
		"F2 会话列表 · Alt+←/→ 切换 · Alt+N 新会话\n" +
		"Ctrl+O 完整执行记录 · Esc 返回聊天\n" +
		"滚轮 / PgUp/PgDown 查看记录 · Ctrl+Home 顶部 · Ctrl+End 最新\n" +
		"Ctrl+C 停止当前任务/退出\n" +
		"菜单：↑↓ 选择 · Enter 确认 · Esc 返回\n" +
		"/config：Tab / ↑↓ 切换字段 · Esc 放弃修改\n" +
		"/resume <会话ID> 也可直接恢复指定会话"
}
