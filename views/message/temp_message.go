package message

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	imp "github.com/whxcode/mini-gnet/interface"
)

type TempMessage struct {
	sessionID string
	fd        *os.File
	mtx       sync.Mutex
	chats     []openai.ChatCompletionMessage
}

var _ imp.Message = (*TempMessage)(nil)

func (t *TempMessage) ToChatCompletionMessage() (result []openai.ChatCompletionMessage) {
	return append(result, t.chats...)
}

func (t *TempMessage) append(message openai.ChatCompletionMessage) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.chats = append(t.chats, message)
	b, err := json.Marshal(message)
	if err == nil {
		_, _ = t.fd.Write(append(b, '\n'))
		_ = t.fd.Sync()
	}
}

func (t *TempMessage) AppendSystemMessage(content string) {
	t.append(openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: content})
}

func (t *TempMessage) AppendUserMessage(content string) {
	t.append(openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: content})
}

func (t *TempMessage) AppendAssistant(content string, toolCalls []openai.ToolCall) {
	t.append(openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant, Content: content, ToolCalls: toolCalls,
	})
}

func (t *TempMessage) AppendTool(toolCallID, content string) {
	t.append(openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleTool, ToolCallID: toolCallID, Content: content,
	})
}

func MakeTempMessage() *TempMessage {
	t := &TempMessage{
		sessionID: uuid.NewString(),
	}

	fd, err := os.OpenFile(config.DefautConfig.ChatsDir+"/"+t.sessionID+".jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		panic(err)
	}

	t.fd = fd

	return t
}
