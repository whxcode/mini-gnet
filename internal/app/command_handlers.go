package app

import (
	"os"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/views/commands"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/message"
)

// handleHelp 展示从注册表生成的帮助；菜单关闭后直接返回主循环。
func handleHelp(_ *App, _ []string) error {
	return commands.ShowHelp(commandHelp())
}

// handleConfig 完成配置编辑、保存和应用；Esc 不修改状态，保存失败保留原客户端和会话。
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
	if replacement != nil {
		a.session.Close()
		a.session = replacement
	}
	a.config = next
	a.client.BaseURL, a.client.ApiKey, a.client.Model = next.BaseURL, next.APIKey, next.Model
	path, _ := config.Path()
	text := "配置已保存并应用：" + path + "\n模型：" + a.config.Model
	if replacement != nil {
		text += "\n目录已改变，已开启新会话：" + replacement.SessionID
	}
	conversation.Print(conversation.Event{Role: conversation.System, Text: text})
	return nil
}

// handleResume 完成历史选择、加载和会话替换；读取成功后才关闭旧会话。
func handleResume(a *App, args []string) error {
	var id string
	if len(args) == 1 {
		id = args[0]
	} else {
		var err error
		id, err = commands.SelectSession(a.config.ChatsDir, a.session.SessionID)
		if err != nil {
			return err
		}
		// Esc 返回，不改变当前会话。
		if id == "" {
			return nil
		}
	}
	if id == a.session.SessionID {
		conversation.Print(conversation.Event{Role: conversation.System, Text: "当前已经是该会话。"})
		return nil
	}
	next, err := message.Resume(a.config.ChatsDir, id)
	if err != nil {
		return err
	}
	a.session.Close()
	a.session = next
	conversation.Print(conversation.Event{Role: conversation.System, Text: "当前会话：" + next.SessionID})
	showHistory(next)
	return nil
}

// handleNew 创建并切换新会话；创建失败时继续使用原会话。
func handleNew(a *App, _ []string) error {
	next, err := startSession(a.config)
	if err != nil {
		return err
	}
	a.session.Close()
	a.session = next
	conversation.Print(conversation.Event{Role: conversation.System, Text: "当前会话：" + next.SessionID})
	return nil
}

// handleExit 设置应用退出状态，主循环结束后由 main 统一释放资源。
func handleExit(a *App, _ []string) error {
	a.quit = true
	return nil
}
