// Package commands 只负责内置命令的菜单显示；命令注册和状态修改位于 internal/app。
package commands

import (
	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/views/message"
)

// ShowHelp 在交互入口显示帮助菜单，Esc 关闭；管道文本输出由 RunPipe 负责。
func ShowHelp(text string) error {
	_, err := tea.NewProgram(newHelp(text)).Run()
	return err
}

// EditConfig 编辑配置副本；确认返回新配置，Esc 返回 nil，菜单本身不保存文件。
func EditConfig(cfg config.Config) (*config.Config, error) {
	model, err := tea.NewProgram(newConfig(cfg)).Run()
	if err != nil {
		return nil, err
	}
	return model.(configModel).saved, nil
}

// SelectSession 列出指定目录中的历史会话；确认返回 ID，Esc 返回空字符串。
func SelectSession(chatsDir, currentID string) (string, error) {
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

// SelectOpenSession 选择已打开的会话，编号由应用分配；Esc 返回空字符串。
func SelectOpenSession(sessions []message.SessionInfo, currentID string) (string, error) {
	m := newResume(sessions, currentID)
	m.title = "切换会话（后台任务继续执行）"
	m.action = "切换"
	for i, s := range sessions {
		if s.ID == currentID {
			m.cursor = i
			break
		}
	}
	model, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	return model.(resumeModel).selected, nil
}

// ConfirmExit 在还有后台任务时确认退出，Esc 不停止任务。
func ConfirmExit(running int) (bool, error) {
	model, err := tea.NewProgram(exitModel{running: running}).Run()
	if err != nil {
		return false, err
	}
	return model.(exitModel).confirmed, nil
}
