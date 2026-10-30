package app

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/textarea"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/input"
	"github.com/whxcode/mini-gnet/views/message"
)

// Session 是一个打开的聊天窗口：保存聊天记录、输入草稿和这个窗口的任务状态。
// App.CurrentSession 决定看哪个窗口；切换窗口不会移动或重启任务。
type Session struct {
	history *message.TempMessage // 模型上下文及其 JSONL 文件句柄，负责聊天记录的持久化。

	// 输入框只由界面操作，后台任务不修改用户正在编辑的文字。
	draft        textarea.Model // 未发送的输入，切换会话时保留文字和光标。
	scrollOffset int            // 向上查看历史的行数；0 表示查看最新消息。

	// 以下字段会被后台任务写入、界面读取，读写时都要持有 mu。
	mu              sync.Mutex           // 防止界面读取到后台更新一半的状态；不锁项目文件。
	taskQueue       chan *task           // 本会话的任务队列，只由一个 Run 循环消费。
	workerDone      chan struct{}        // Run 退出后关闭，退出程序时等待它。
	closed          bool                 // 关闭后拒绝入队，不再启动新任务。
	active          *task                // 当前任务；nil 表示没有正在执行的任务。
	pending         []*task              // 尚未开始的任务，用于显示和取消，包含已出队但尚未取得锁的任务。
	status          string               // 界面标签，例如“空闲”“执行中”“已完成”“失败”。
	title           string               // 会话列表中的标题，默认取第一条用户提问。
	messages        []conversation.Event // 显示用的完整消息；不含内部 system 提示词，可含运行错误。
	streamText      string               // 当前回答尚未结束的文字，随着模型流逐段累积。
	progressText    string               // 当前在做什么，例如“正在生成”或“run_python 正在执行”。
	hasUnreadResult bool                 // 本轮已有结果但还没查看，用于后台完成提醒。
	updatedAt       time.Time            // 最近一次状态或文字更新的时间，用于会话列表。
}

// newSession 根据聊天记录创建一个窗口；加载历史只恢复显示，不执行历史任务。
func newSession(history *message.TempMessage) *Session {
	session := &Session{
		history:    history,
		taskQueue:  make(chan *task, 16),
		workerDone: make(chan struct{}),
		draft:      input.NewTextArea(),
		status:     "空闲",
		title:      "新会话",
		updatedAt:  time.Now(),
		messages:   historyEvents(history),
	}

	for _, message := range session.messages {
		if message.Role == conversation.User {
			session.title = strings.Join(strings.Fields(message.Text), " ")
			break
		}
	}

	return session
}

// Run 是会话唯一的消费循环：取一个任务、标记开始、执行到底，再取下一个。
// attachSession 只启动一次；切换会话不启动新的消费协程。
func (s *Session) Run() {
	defer close(s.workerDone)
	for job := range s.taskQueue {
		// 与入队和停止共用一把锁，避免 /stop 漏掉刚出队、尚未开始的任务。
		s.mu.Lock()
		if index := slices.Index(s.pending, job); index >= 0 {
			s.pending = slices.Delete(s.pending, index, index+1)
		}
		if s.closed {
			job.cancel()
		}
		if err := job.ctx.Err(); err != nil {
			job.err = err
			job.cancel()
			close(job.done)
			s.mu.Unlock()
			continue
		}
		s.active = job
		s.status = "执行中"
		s.streamText = ""
		s.progressText = "正在请求模型…"
		s.hasUnreadResult = false
		s.updatedAt = time.Now()
		s.mu.Unlock()

		// 不加 go：上一项真正结束后才开始下一项，同一会话的上下文不会交叉。
		s.RunTask(job)
	}
}

// stop 取消当前任务和所有待执行任务；只影响本会话，之后仍可提交新任务。
func (s *Session) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != nil {
		s.status = "正在停止"
		s.active.cancel()
	} else if len(s.pending) > 0 {
		s.status = "已停止"
	}
	count := len(s.pending)
	for _, job := range s.pending {
		job.cancel()
	}
	// 立即腾空缓冲区。已被 Run 取走的任务由 Run 检查取消信号并结束，避免重复关闭 done。
drain:
	for {
		select {
		case job, ok := <-s.taskQueue:
			if !ok {
				break drain
			}
			job.err = context.Canceled
			close(job.done)
		default:
			break drain
		}
	}
	s.pending = nil
	if count > 0 {
		s.messages = append(s.messages, conversation.Event{Role: conversation.System, Text: "已取消本会话的待执行任务。"})
	}
	s.updatedAt = time.Now()
}
