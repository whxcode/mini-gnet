package app

import (
	"fmt"
	"path/filepath"
	"strconv"

	sessions "github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/message"
)

// AttachSession 将新建或恢复的记录加入列表，不关闭其他会话，也不停止它们的任务。
func (a *App) AttachSession(record *message.TempMessage) {
	s := sessions.New(record, a.agent)
	a.opened = append(a.opened, s)
	a.ActivateSession(s)
	go s.Run() // 每个会话只启动一个消费循环；/ps 切换不会走到这里。
}

// ActivateSession 只改变当前查看对象；后台任务继续使用所属会话的记录和启动时的配置。
func (a *App) ActivateSession(s *sessions.Session) {
	a.CurrentSession = s
	s.MarkRead()
}

// FindSession 支持使用列表序号或完整会话 ID 定位已打开的会话。
func (a *App) FindSession(value string) (*sessions.Session, error) {
	if number, err := strconv.Atoi(value); err == nil && number > 0 && number <= len(a.opened) {
		return a.opened[number-1], nil
	}

	var found *sessions.Session

	for _, s := range a.opened {
		if s.History.SessionID == value {
			if found != nil {
				return nil, fmt.Errorf("多个目录中存在相同会话 ID，请使用列表序号")
			}

			found = s
		}
	}

	if found == nil {
		return nil, fmt.Errorf("未找到已打开的会话 %s，输入 /sessions 查看列表", value)
	}

	return found, nil
}

// OpenedHistory 检查历史文件是否已打开，防止 /resume 为同一记录再次创建写入者。
func (a *App) OpenedHistory(dir, id string) *sessions.Session {
	want, _ := filepath.Abs(filepath.Join(dir, id+".jsonl"))

	for _, s := range a.opened {
		path, _ := filepath.Abs(s.History.Path())

		if path == want {
			return s
		}
	}

	return nil
}

// RunningCount 统计有执行或等待任务的会话数，用于退出确认。
func (a *App) RunningCount() int {
	count := 0

	for _, s := range a.opened {
		state := s.Snapshot()

		if state.Running || len(state.Pending) > 0 {
			count++
		}
	}

	return count
}

// Notify 将运行提示放入当前会话；输出由当前入口负责，不在这里操作终端。
func (a *App) Notify(text string) {
	event := conversation.Event{
		Role: conversation.System,
		Text: text,
	}
	a.CurrentSession.Output.Update(event)
}

// reportError 记录当前操作的错误，普通输入错误不停止其他会话的后台任务。
func (a *App) reportError(err error) {
	event := conversation.Event{
		Role: conversation.Error,
		Text: err.Error(),
	}
	a.CurrentSession.Output.Update(event)
}
