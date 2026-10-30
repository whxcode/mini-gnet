package app

import (
	"os"
	"strconv"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/views/commands"
	"github.com/whxcode/mini-gnet/views/message"
)

// handleHelp 展示从注册表生成的帮助；菜单关闭后直接返回主循环。
func handleHelp(_ *App, _ []string) error {
	return commands.ShowHelp(commandHelp())
}

// handleConfig 完成配置编辑、保存和应用；Esc 不修改状态，保存失败保留原配置和会话。
func handleConfig(a *App, _ []string) error {
	edited, err := commands.EditConfig(a.config)
	if err != nil {
		return err
	}
	// 用户按 Esc，结束命令即可。
	if edited == nil {
		return nil
	}
	next := *edited
	if next.ProjectDir == "" {
		var err error
		next.ProjectDir, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	var replacement *message.TempMessage
	// 改变目录时开启新会话，避免把新项目的问题混进旧项目的记录。
	if next.ChatsDir != a.config.ChatsDir || next.ProjectDir != a.config.ProjectDir {
		var err error
		replacement, err = startSession(next)
		if err != nil {
			return err
		}
	}
	if err := config.Save(*edited); err != nil {
		if replacement != nil {
			replacement.Close()
		}
		return err
	}
	// 所有会话的后续任务都读取这份配置，已启动任务继续使用启动时的参数。
	a.config = next
	if replacement != nil {
		a.attachSession(replacement)
	}
	path, _ := config.Path()
	text := "配置已保存并应用：" + path + "\n模型：" + a.config.Model
	if replacement != nil {
		text += "\n目录已改变，已开启新会话：" + replacement.SessionID
	}
	a.notify(text)
	return nil
}

// handleResume 恢复历史或切回已打开的会话；后台任务和旧记录保持打开。
func handleResume(a *App, args []string) error {
	var id string
	if len(args) == 1 {
		id = args[0]
	} else {
		var err error
		id, err = commands.SelectSession(a.config.ChatsDir, a.CurrentSession.history.SessionID)
		if err != nil {
			return err
		}
		// Esc 返回，不改变当前会话。
		if id == "" {
			return nil
		}
	}
	if opened := a.openedHistory(a.config.ChatsDir, id); opened != nil {
		a.activateSession(opened)
		return nil
	}
	next, err := message.Resume(a.config.ChatsDir, id)
	if err != nil {
		return err
	}
	a.attachSession(next)
	return nil
}

// handleNew 创建并切换新会话；创建失败时继续使用原会话。
func handleNew(a *App, _ []string) error {
	next, err := startSession(a.config)
	if err != nil {
		return err
	}
	a.attachSession(next)
	return nil
}

// handleExit 设置应用退出状态，主循环结束后由 main 统一释放资源。
func handleExit(a *App, _ []string) error {
	if count := a.runningCount(); count > 0 {
		confirmed, err := commands.ConfirmExit(count)
		if err != nil {
			return err
		}
		if !confirmed {
			return nil
		}
	}
	a.quit = true
	return nil
}

// handleSessions 列出已打开会话或按编号/ID 切换，始终保留旧任务及输入草稿。
func handleSessions(a *App, args []string) error {
	current := a.CurrentSession
	if len(args) == 1 {
		value := args[0]
		if value == "next" || value == "prev" {
			for i, s := range a.opened {
				if s == current {
					delta := 1
					if value == "prev" {
						delta = -1
					}
					a.activateSession(a.opened[(i+delta+len(a.opened))%len(a.opened)])
					return nil
				}
			}
		}
		selected, err := a.findSession(value)
		if err != nil {
			return err
		}
		a.activateSession(selected)
		return nil
	}
	var entries []message.SessionInfo
	var currentID string
	for i, s := range a.opened {
		snapshot := s.snapshot()
		id := strconv.Itoa(i + 1)
		if s == current {
			currentID = id
		}
		title := snapshot.Status + " · " + snapshot.Title
		if snapshot.HasUnreadResult && s != current {
			title += " [有新结果]"
		}
		entries = append(entries, message.SessionInfo{ID: id, Title: title, UpdatedAt: snapshot.UpdatedAt, Count: len(snapshot.Messages)})
	}
	id, err := commands.SelectOpenSession(entries, currentID)
	if err != nil || id == "" {
		return err
	}
	selected, err := a.findSession(id)
	if err != nil {
		return err
	}
	a.activateSession(selected)
	return nil
}

// handleStop 只取消当前会话的任务；其他会话继续执行。
func handleStop(a *App, _ []string) error {
	s := a.CurrentSession
	if !s.snapshot().Running {
		a.notify("当前会话没有正在执行的任务。")
		return nil
	}
	s.stop()
	return nil
}
