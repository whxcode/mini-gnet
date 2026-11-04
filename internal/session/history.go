package session

import (
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/agent"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/message"
)

// NewHistory 创建记录文件并保存当前工作目录对应的系统提示词。
func NewHistory(cfg config.Config) (*message.TempMessage, error) {
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
	history := session.Records()
	events := []conversation.Event{}
	toolNames := make(map[string]string)
	callIndexes := make(map[string]int)

	for _, msg := range history {
		for _, call := range msg.ToolCalls {
			toolNames[call.ID] = call.Function.Name
		}

		switch msg.Role {
		case "user":
			events = append(events, conversation.Event{
				Role: conversation.User,
				Name: conversation.Username(),
				Text: msg.Content,
			})

		case "assistant":
			stage := agent.Stage("") // 旧记录没有 stop 标记，不冒充已确认的最终回答。

			if len(msg.ToolCalls) > 0 || msg.Response != nil {
				stage = agent.Process
			}

			if msg.Response != nil && msg.Response.Final {
				stage = agent.Final
			}

			if msg.Content != "" || stage == agent.Final {
				events = append(events, conversation.Event{
					Role:  conversation.Assistant,
					Text:  msg.Content,
					Stage: stage,
				})
			}

			for _, call := range msg.ToolCalls {
				callIndexes[call.ID] = len(events)
				events = append(events, conversation.Event{
					Role: conversation.Tool,
					Name: call.Function.Name + " · 调用",
					Text: tools.CallText(call.Function.Name, call.ID, call.Function.Arguments),
				})
			}

		case "tool":
			if index, exists := callIndexes[msg.ToolCallID]; exists {
				events[index].Text += tools.ExecutionText(msg.Execution)
			}

			name := toolNames[msg.ToolCallID]
			events = append(events, conversation.Event{
				Role: conversation.Tool,
				Name: name + " · 结果",
				Text: tools.ResultText(msg.ToolCallID, msg.Content),
			})
		}
	}

	return events
}
