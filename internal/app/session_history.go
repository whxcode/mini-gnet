package app

import (
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
	if err := session.AppendSystemMessage(prompt.CheckAgentPrompt(cfg.ProjectDir)); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

// historyEvents 将完整历史转换为显示消息，过滤内部提示词，恢复工具调用对应的名称。
func historyEvents(session *message.TempMessage) []conversation.Event {
	history := session.ToChatCompletionMessage()
	events := []conversation.Event{}
	toolNames := make(map[string]string)
	for _, msg := range history {
		for _, call := range msg.ToolCalls {
			toolNames[call.ID] = call.Function.Name
		}
		switch msg.Role {
		case "user":
			events = append(events, conversation.Event{Role: conversation.User, Name: conversation.Username(), Text: msg.Content})
		case "assistant":
			if msg.Content != "" {
				events = append(events, conversation.Event{Role: conversation.Assistant, Text: msg.Content})
			}
			for _, call := range msg.ToolCalls {
				events = append(events, conversation.Event{Role: conversation.Tool, Name: call.Function.Name + " · 调用", Text: toolInputText(call.Function.Name, call.Function.Arguments)})
			}
		case "tool":
			name := toolNames[msg.ToolCallID]
			events = append(events, conversation.Event{Role: conversation.Tool, Name: name + " · 结果", Text: toolOutputText(name, msg.Content)})
		}
	}
	return events
}
