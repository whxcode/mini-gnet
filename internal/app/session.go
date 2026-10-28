package app

import (
	"fmt"

	"github.com/whxcode/mini-gnet/config"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
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
