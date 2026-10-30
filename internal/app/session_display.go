package app

import (
	"strings"
	"time"

	"github.com/whxcode/mini-gnet/views/conversation"
)

// sessionSnapshot 是界面读取的一份副本，没有模型客户端或文件句柄。
// 复制时短暂加锁，界面随后可以慢慢排版，不会一直挡住后台任务。
type sessionSnapshot struct {
	ID              string               // 聊天记录 ID。
	Title           string               // 会话标题。
	Status          string               // 任务状态标签。
	Running         bool                 // 是否有任务正在执行。
	Pending         []string             // 等待执行的提问，与模型聊天历史分开显示。
	Messages        []conversation.Event // 完整显示消息的副本。
	StreamText      string               // 正在生成、尚未结束的回答。
	ProgressText    string               // 当前执行步骤的提示。
	HasUnreadResult bool                 // 是否需要提示有新结果。
	UpdatedAt       time.Time            // 最近更新时间。
}

// snapshot 由界面调用，在同一把锁内复制显示字段，避免一边读一边被后台修改。
func (s *Session) snapshot() sessionSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending := make([]string, 0, len(s.pending))
	for _, job := range s.pending {
		if job.ctx.Err() == nil {
			pending = append(pending, job.query)
		}
	}
	return sessionSnapshot{
		ID:              s.history.SessionID,
		Title:           s.title,
		Status:          s.status,
		Running:         s.active != nil,
		Pending:         pending,
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
