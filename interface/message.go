package imp

import "github.com/sashabaranov/go-openai"

type Message interface {
	AppendSystemMessage(content string)
	AppendUserMessage(content string)
	AppendAssistant(content string, toolCalls []openai.ToolCall)
	AppendTool(toolCallID, content string)
	ToChatCompletionMessage() []openai.ChatCompletionMessage
}
