// Package commands 只负责内置命令的菜单显示；命令注册和状态修改位于 internal/app。
package commands

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/views/message"
)

// interactive 判断输入输出是否都是终端，避免在管道中启动交互菜单。
func interactive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// ShowHelp 显示调用方提供的帮助文本；管道模式直接打印，Esc 关闭菜单。
func ShowHelp(text string) error {
	if !interactive() {
		fmt.Println(text)
		return nil
	}
	_, err := tea.NewProgram(newHelp(text)).Run()
	return err
}

// EditConfig 编辑配置副本；确认返回新配置，Esc 返回 nil，菜单本身不保存文件。
func EditConfig(cfg config.Config) (*config.Config, error) {
	if !interactive() {
		return nil, fmt.Errorf("请在交互终端中使用 /config 编辑配置")
	}
	model, err := tea.NewProgram(newConfig(cfg)).Run()
	if err != nil {
		return nil, err
	}
	return model.(configModel).saved, nil
}

// SelectSession 列出指定目录中的历史会话；确认返回 ID，Esc 返回空字符串。
func SelectSession(chatsDir, currentID string) (string, error) {
	if !interactive() {
		return "", fmt.Errorf("管道中请使用 /resume <会话ID>；交互终端中可用上下键选择")
	}
	sessions, err := message.List(chatsDir)
	if err != nil {
		return "", err
	}
	model, err := tea.NewProgram(newResume(sessions, currentID)).Run()
	if err != nil {
		return "", err
	}
	return model.(resumeModel).selected, nil
}
