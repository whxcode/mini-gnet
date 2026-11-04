package agent

import (
	"strings"
	"sync"
	"time"
)

// Output 是一份对话的输出记录，Agent 直接写入，界面读取；不使用回调或消息转发协程。
type Output struct {
	mu    sync.Mutex     // 保护生成中的文字和完整消息。
	state OutputSnapshot // 当前输出内容。
}

// OutputSnapshot 是输出副本，不包含运行状态、队列或取消入口。
type OutputSnapshot struct {
	Messages     []Event   // 完整显示消息。
	StreamText   string    // 正在生成的文字。
	ProgressText string    // 当前执行步骤。
	UpdatedAt    time.Time // 最近一次输出时间。
}

// NewOutput 用历史消息初始化输出，不重新执行任何操作。
func NewOutput(messages []Event) *Output {
	return &Output{
		state: OutputSnapshot{
			Messages:  append([]Event(nil), messages...),
			UpdatedAt: time.Now(),
		},
	}
}

// Update 直接记录事件；完整消息返回位置，供工具结束后补充该条调用的执行信息。
func (o *Output) Update(event Event) int {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.state.UpdatedAt = time.Now()

	switch event.Kind {
	case Status:
		o.state.ProgressText = strings.TrimSpace(event.Name + " " + event.Text)
	case Delta:
		o.state.StreamText += event.Text
	case Message:
		if event.Role == Assistant {
			o.state.StreamText = ""

			if event.Stage == Process && strings.TrimSpace(event.Text) == "" {
				return -1 // 工具轮次可能没有正文，清空预览即可，不创建空白消息。
			}
		}

		o.state.Messages = append(o.state.Messages, event)

		return len(o.state.Messages) - 1
	}

	return -1
}

// ReplaceMessage 更新已有消息，保留位置；调用编号重复时也不会覆盖其他轮次。
func (o *Output) ReplaceMessage(index int, event Event) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if index < 0 || index >= len(o.state.Messages) {
		return
	}

	o.state.Messages[index] = event
	o.state.UpdatedAt = time.Now()
}

// Snapshot 复制输出，调用者可以在解锁后进行排版。
func (o *Output) Snapshot() OutputSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()

	state := o.state
	state.Messages = append([]Event(nil), state.Messages...)

	return state
}

// Reset 在下一项开始时清理流式预览，保留已有完整消息。
func (o *Output) Reset() {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.state.StreamText = ""
	o.state.ProgressText = "正在请求模型…"
}

// Finish 在本项结束时保留中断的文字，再清除生成中的预览。
func (o *Output) Finish() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.state.StreamText != "" {
		o.state.Messages = append(o.state.Messages, Event{
			Role:  Assistant,
			Name:  "未完成",
			Text:  o.state.StreamText,
			Stage: Process,
		})
	}

	o.state.StreamText = ""
	o.state.ProgressText = ""
}
