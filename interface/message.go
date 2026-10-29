package imp

import "github.com/sashabaranov/go-openai"

// Message 保存模型上下文，每次追加都直接返回本次操作的错误。
type Message interface {
	AppendSystemMessage(content string) error
	AppendUserMessage(content string) error
	AppendAssistant(content string, toolCalls []openai.ToolCall) error
	AppendTool(toolCallID, content string) error
	ToChatCompletionMessage() []openai.ChatCompletionMessage
}
