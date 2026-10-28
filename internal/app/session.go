package app

import (
	"fmt"
	"os"

	"github.com/whxcode/mini-gnet/config"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
	"github.com/whxcode/mini-gnet/views/commands"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/message"
)

// startSession 创建记录文件并保存当前工作目录对应的系统提示词。
func startSession(cfg config.Config) (*message.TempMessage, error) {
	session, err := message.New(cfg.ChatsDir)
	if err != nil {
		return nil, err
	}
	session.AppendSystemMessage(prompt.CheckAgentPrompt(cfg.ProjectDir))
	if err := session.Err(); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

// applyLocalCommand 在菜单确认后才改变配置或会话，Esc 返回的空结果不会触发任何修改。
func (a *App) applyLocalCommand(result commands.Result) error {
	if result.Config != nil {
		next := *result.Config
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
		if err := config.Save(*result.Config); err != nil {
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
	}
	if result.New || result.SessionID != "" {
		if result.SessionID == a.session.SessionID {
			conversation.Print(conversation.Event{Role: conversation.System, Text: "当前已经是该会话。"})
			return nil
		}
		var next *message.TempMessage
		var err error
		if result.New {
			next, err = startSession(a.config)
		} else {
			next, err = message.Resume(a.config.ChatsDir, result.SessionID)
		}
		if err != nil {
			return err
		}
		a.session.Close()
		a.session = next
		conversation.Print(conversation.Event{Role: conversation.System, Text: "当前会话：" + next.SessionID})
		if !result.New {
			showHistory(next)
		}
	}
	return nil
}

// showHistory 回显最近的对话；内部提示词不显示，模型上下文仍保留完整记录。
func showHistory(session *message.TempMessage) {
	history := session.ToChatCompletionMessage()
	// 屏幕仅回显最近 20 条；模型仍会收到完整历史，不会因为显示截取而遗忘上下文。
	start := max(0, len(history)-20)
	if start > 0 {
		conversation.Print(conversation.Event{Role: conversation.System, Text: fmt.Sprintf("已加载完整历史，下面显示最近 20 条（共 %d 条）。", len(history))})
	}
	toolNames := make(map[string]string)
	for i, msg := range history {
		for _, call := range msg.ToolCalls {
			toolNames[call.ID] = call.Function.Name
		}
		if i < start || msg.Role == "system" {
			continue
		}
		switch msg.Role {
		case "user":
			conversation.Print(conversation.Event{Role: conversation.User, Name: conversation.Username(), Text: msg.Content})
		case "assistant":
			if msg.Content != "" {
				conversation.Print(conversation.Event{Role: conversation.Assistant, Text: msg.Content})
			}
			for _, call := range msg.ToolCalls {
				conversation.Print(conversation.Event{Role: conversation.Tool, Name: call.Function.Name + " · 调用", Text: toolInputText(call.Function.Name, call.Function.Arguments)})
			}
		case "tool":
			name := toolNames[msg.ToolCallID]
			conversation.Print(conversation.Event{Role: conversation.Tool, Name: name + " · 结果", Text: toolOutputText(name, msg.Content)})
		}
	}
}
