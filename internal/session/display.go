package session

import (
	"time"

	"github.com/whxcode/mini-gnet/views/conversation"
)

// Snapshot 是界面读取的一份副本，没有模型客户端或文件句柄。
// 复制时短暂加锁，界面随后可以慢慢排版，不会一直挡住后台任务。
type Snapshot struct {
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

// Snapshot 由界面调用，在同一把锁内复制显示字段，避免一边读一边被后台修改。
func (s *Session) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	pending := make([]string, 0, len(s.pending))

	for _, job := range s.pending {
		if job.Context.Err() == nil {
			pending = append(pending, job.Query)
		}
	}

	output := s.Output.Snapshot()
	updated := s.updatedAt

	if output.UpdatedAt.After(updated) {
		updated = output.UpdatedAt
	}

	return Snapshot{
		ID:              s.History.SessionID,
		Title:           s.title,
		Status:          s.status,
		Running:         s.active != nil,
		Pending:         pending,
		Messages:        output.Messages,
		StreamText:      output.StreamText,
		ProgressText:    output.ProgressText,
		HasUnreadResult: s.hasUnreadResult,
		UpdatedAt:       updated,
	}
}

// MarkRead 由界面在查看本会话时调用，相当于清掉聊天窗口的“有新结果”提示。
// 它不读取历史文件，也不启动任务。
func (s *Session) MarkRead() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.hasUnreadResult = false
}
