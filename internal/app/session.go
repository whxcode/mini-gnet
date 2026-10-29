package app

import (
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textarea"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/net"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/input"
	"github.com/whxcode/mini-gnet/views/message"
)

// Session 是一个打开的聊天窗口：保存聊天记录、输入草稿和这个窗口的任务状态。
// App.CurrentSession 决定看哪个窗口；切换窗口不会移动或重启任务。
type Session struct {
	// 会话配置只由主循环修改。任务启动前会复制客户端和工作目录。
	history *message.TempMessage // 模型上下文及其 JSONL 文件句柄，负责聊天记录的持久化。
	config  config.Config        // 此会话后续请求使用的配置。
	client  net.Client           // 按此会话配置创建的模型客户端。

	// 输入框只由界面操作，后台任务不修改用户正在编辑的文字。
	draft        textarea.Model // 未发送的输入，切换会话时保留文字和光标。
	scrollOffset int            // 向上查看历史的行数；0 表示查看最新消息。

	// 以下字段会被后台任务写入、界面读取，读写时都要持有 mu。
	mu              sync.Mutex           // 防止界面读取到后台更新一半的状态；不锁项目文件。
	task            *task                // 最近一次任务，用于停止它或在退出时等待它结束；初始为 nil。
	running         bool                 // 是否正在执行，防止同一会话同时接受两次提问。
	status          string               // 界面标签，例如“空闲”“执行中”“已完成”“失败”。
	title           string               // 会话列表中的标题，默认取第一条用户提问。
	messages        []conversation.Event // 显示用的完整消息；不含内部 system 提示词，可含运行错误。
	streamText      string               // 当前回答尚未结束的文字，随着模型流逐段累积。
	progressText    string               // 当前在做什么，例如“正在生成”或“run_python 正在执行”。
	hasUnreadResult bool                 // 本轮已有结果但还没查看，用于后台完成提醒。
	updatedAt       time.Time            // 最近一次状态或文字更新的时间，用于会话列表。
}

// sessionSnapshot 是界面读取的一份副本，没有模型客户端或文件句柄。
// 复制时短暂加锁，界面随后可以慢慢排版，不会一直挡住后台任务。
type sessionSnapshot struct {
	ID              string               // 聊天记录 ID。
	Title           string               // 会话标题。
	Status          string               // 任务状态标签。
	Running         bool                 // 是否有任务正在执行。
	Messages        []conversation.Event // 完整显示消息的副本。
	StreamText      string               // 正在生成、尚未结束的回答。
	ProgressText    string               // 当前执行步骤的提示。
	HasUnreadResult bool                 // 是否需要提示有新结果。
	UpdatedAt       time.Time            // 最近更新时间。
}

// newSession 根据聊天记录创建一个窗口；加载历史只恢复显示，不执行历史任务。
func newSession(history *message.TempMessage, cfg config.Config, client net.Client) *Session {
	session := &Session{
		history:   history,
		config:    cfg,
		client:    client,
		draft:     input.NewTextArea(),
		status:    "空闲",
		title:     "新会话",
		updatedAt: time.Now(),
		messages:  historyEvents(history),
	}
	for _, message := range session.messages {
		if message.Role == conversation.User {
			session.title = strings.Join(strings.Fields(message.Text), " ")
			break
		}
	}
	return session
}

// snapshot 由界面调用，在同一把锁内复制显示字段，避免一边读一边被后台修改。
func (s *Session) snapshot() sessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	return sessionSnapshot{
		ID:              s.history.SessionID,
		Title:           s.title,
		Status:          s.status,
		Running:         s.running,
		Messages:        append([]conversation.Event(nil), s.messages...),
		StreamText:      s.streamText,
		ProgressText:    s.progressText,
		HasUnreadResult: s.hasUnreadResult,
		UpdatedAt:       s.updatedAt,
	}
}

// markRead 由界面在查看本会话时调用，相当于清掉聊天窗口的“有新结果”提示。
// 它不读取历史文件，也不启动任务。
func (s *Session) markRead() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hasUnreadResult = false
}

// updateDisplay 把 Agent 给出的状态、文字片段或完整消息更新到本会话的显示区。
// 这里只改变内存中的显示；保存模型上下文由 RunTask 中的 history.Append... 完成。
func (s *Session) updateDisplay(event conversation.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.updatedAt = time.Now()
	switch event.Kind {
	case conversation.Status:
		// 例如“正在执行工具”：替换进度提示，不当作聊天正文追加。
		s.progressText = strings.TrimSpace(event.Name + " " + event.Text)
	case conversation.Delta:
		// 例如先收到“你”，再收到“好”：拼成正在生成的“你好”。
		s.streamText += event.Text
	case conversation.Message:
		// 完整回答到了，清掉生成中的预览，避免同一句话显示两遍。
		if event.Role == conversation.Assistant {
			s.streamText = ""
		}
		s.messages = append(s.messages, event)
	}
}

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
