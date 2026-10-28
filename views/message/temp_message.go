package message

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	imp "github.com/whxcode/mini-gnet/interface"
)

type TempMessage struct {
	SessionID string
	fd        *os.File
	mtx       sync.Mutex
	chats     []openai.ChatCompletionMessage
	writeErr  error
}

var _ imp.Message = (*TempMessage)(nil)

func (t *TempMessage) ToChatCompletionMessage() (result []openai.ChatCompletionMessage) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return append(result, t.chats...)
}

func (t *TempMessage) append(message openai.ChatCompletionMessage) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.writeErr != nil {
		return
	}
	b, err := json.Marshal(message)
	if err == nil {
		_, err = t.fd.Write(append(b, '\n'))
	}
	if err == nil {
		err = t.fd.Sync()
	}
	if err != nil {
		t.writeErr = err
		return
	}
	t.chats = append(t.chats, message)
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
	t, err := New(config.DefautConfig.ChatsDir)
	if err != nil {
		panic(err)
	}
	return t
}

// New 显式接收记录目录，避免运行时修改配置后还写入旧的全局目录。
func New(chatsDir string) (*TempMessage, error) {
	if err := os.MkdirAll(chatsDir, 0o700); err != nil {
		return nil, err
	}
	id := uuid.NewString()
	fd, err := os.OpenFile(filepath.Join(chatsDir, id+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &TempMessage{SessionID: id, fd: fd}, nil
}

func (t *TempMessage) Close() error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.fd.Close()
}

// Append 系列保留原接口；主循环可通过 Err 检查是否发生保存失败。
func (t *TempMessage) Err() error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.writeErr
}
