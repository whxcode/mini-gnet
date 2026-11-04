package message

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	imp "github.com/whxcode/mini-gnet/interface"
	"github.com/whxcode/mini-gnet/tools"
)

// TempMessage 将会话消息同时保存到 JSONL 文件和模型使用的内存上下文。
type TempMessage struct {
	SessionID string     // 会话编号，也是记录文件名。
	fd        *os.File   // 当前会话的记录文件。
	mtx       sync.Mutex // 保护文件写入和内存消息。
	chats     []Record   // 已成功保存的消息及本地显示标记。
}

// Record 保持原来的 JSONL 消息字段，额外保存响应标记和进程信息；请求模型时只取 SDK 消息。
type Record struct {
	openai.ChatCompletionMessage
	Response  *ResponseInfo        `json:"agent_response,omitempty"` // nil 表示旧格式，未记录结束原因。
	Execution *tools.ExecutionInfo `json:"tool_execution,omitempty"` // 仅保存在本地，不进入模型上下文。
}

// ResponseInfo 记录模型本轮的结束原因和是否为本项最终回答。
type ResponseInfo struct {
	Finish openai.FinishReason `json:"finish_reason"`
	Final  bool                `json:"final"`
}

var _ imp.Message = (*TempMessage)(nil)

// Path 返回已经打开的记录路径；修改应用配置不会把旧会话迁移到另一个文件。
func (t *TempMessage) Path() string {
	return t.fd.Name()
}

// ToChatCompletionMessage 返回消息列表的副本，供模型请求使用。
func (t *TempMessage) ToChatCompletionMessage() (result []openai.ChatCompletionMessage) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	for _, record := range t.chats {
		result = append(result, record.ChatCompletionMessage)
	}

	return result
}

// Records 返回包含显示标记的历史副本，恢复界面时使用，不传给模型。
func (t *TempMessage) Records() []Record {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	return append([]Record(nil), t.chats...)
}

// append 先编码、写文件并同步到磁盘，成功后才更新内存；哪一步失败就直接返回错误。
func (t *TempMessage) append(message Record) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	b, err := json.Marshal(message)
	if err != nil {
		return err
	}

	if _, err := t.fd.Write(append(b, '\n')); err != nil {
		return err
	}

	if err := t.fd.Sync(); err != nil {
		return err
	}

	t.chats = append(t.chats, message)

	return nil
}

// AppendSystemMessage 保存系统提示词，并直接返回保存错误。
func (t *TempMessage) AppendSystemMessage(content string) error {
	return t.append(Record{
		ChatCompletionMessage: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: content,
		},
	})
}

// AppendUserMessage 保存用户提问，并直接返回保存错误。
func (t *TempMessage) AppendUserMessage(content string) error {
	return t.append(Record{
		ChatCompletionMessage: openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: content,
		},
	})
}

// AppendAssistant 保存模型回答及工具调用，并直接返回保存错误。
func (t *TempMessage) AppendAssistant(content string, toolCalls []openai.ToolCall) error {
	return t.append(Record{
		ChatCompletionMessage: openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   content,
			ToolCalls: toolCalls,
		},
	})
}

// AppendResponse 保存模型响应和结束标记；旧的 AppendAssistant 仍可用于没有标记的历史消息。
func (t *TempMessage) AppendResponse(
	content string,
	calls []openai.ToolCall,
	finish openai.FinishReason,
	final bool,
) error {
	return t.append(Record{
		ChatCompletionMessage: openai.ChatCompletionMessage{
			Role:      openai.ChatMessageRoleAssistant,
			Content:   content,
			ToolCalls: calls,
		},
		Response: &ResponseInfo{
			Finish: finish,
			Final:  final,
		},
	})
}

// AppendTool 保存工具结果及其调用编号，并直接返回保存错误。
func (t *TempMessage) AppendTool(toolCallID, content string) error {
	return t.AppendToolExecution(toolCallID, content, nil)
}

// AppendToolExecution 保存工具结果和当时的执行信息，恢复会话时不重新读取工具配置。
func (t *TempMessage) AppendToolExecution(toolCallID, content string, execution *tools.ExecutionInfo) error {
	return t.append(Record{
		ChatCompletionMessage: openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: toolCallID,
			Content:    content,
		},
		Execution: execution,
	})
}

// New 显式接收记录目录，避免运行时修改配置后还写入旧的全局目录。
func New(chatsDir string) (*TempMessage, error) {
	if err := os.MkdirAll(chatsDir, 0o700); err != nil {
		return nil, err
	}

	id := uuid.NewString()
	fd, err := os.OpenFile(
		filepath.Join(chatsDir, id+".jsonl"),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND,
		0o600,
	)
	if err != nil {
		return nil, err
	}

	return &TempMessage{
		SessionID: id,
		fd:        fd,
	}, nil
}

// Close 关闭记录文件；调用方应先等待本会话任务结束。
func (t *TempMessage) Close() error {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	return t.fd.Close()
}
