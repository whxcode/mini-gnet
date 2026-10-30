package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// task 表示一次提问的执行，而 Session 是可反复提问的整个聊天会话。
// 主循环只把 task 入队，Session.Run 顺序执行；提交后这些参数不再修改。
type task struct {
	query  string             // 提交的提问，轮到本任务时才写入聊天历史。
	ctx    context.Context    // 本次执行的取消信号，模型请求和工具调用都使用它。
	cancel context.CancelFunc // /stop 调用它，请求本次执行停止，不影响其他会话。
	config config.Config      // 提交时的配置副本，排队和执行期间不跟随 /config 改变。
	tools  []openai.Tool      // 本次请求允许模型调用的工具声明。
	done   chan struct{}      // 本项执行或取消完成后关闭，供管道和测试等待。
	err    error              // 最终执行错误；其他协程必须等 done 关闭后才能读取。
}

// enqueueTask 只校验输入、固定提交参数并入队，不保存提问、不请求模型、不为单项任务启动协程。
func (a *App) enqueueTask(parent context.Context, query string) (*task, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("问题不能为空")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	s := a.CurrentSession
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("当前会话已关闭")
	}
	if len(s.pending) >= cap(s.taskQueue) {
		return nil, fmt.Errorf("当前会话的等待队列已满（%d 项），请稍后再发送", cap(s.taskQueue))
	}
	ctx, cancel := context.WithCancel(parent)
	job := &task{
		query:  query,
		ctx:    ctx,
		cancel: cancel,
		config: a.config,
		tools:  append([]openai.Tool(nil), a.tools...),
		done:   make(chan struct{}),
	}
	// 使用非阻塞发送：队列满时保留输入，不能让主循环卡在这里。
	select {
	case s.taskQueue <- job:
		s.pending = append(s.pending, job)
		s.updatedAt = time.Now()
		if s.title == "新会话" {
			s.title = strings.Join(strings.Fields(query), " ")
		}
		return job, nil
	default:
		cancel()
		return nil, fmt.Errorf("当前会话的等待队列已满（16 项），请稍后再发送")
	}
}

// finishTask 由 RunTask 退出时调用：保存最终状态，保留中断的文字，并通知等待者任务已结束。
// 所有内存状态和历史写入都结束后才关闭 done，避免程序退出时提前关闭文件。
func (s *Session) finishTask(job *task, runErr error) error {
	defer job.cancel() // 释放本次 context 的资源，正常完成也需要清理。

	// 1. 写入错误已由 RunTask 直接返回；这里只补充任务取消的状态。
	if job.ctx.Err() != nil {
		runErr = job.ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 2. 中途断开时保留已生成的文字供用户看，但不把它当作完整回答保存给模型。
	if s.streamText != "" {
		s.messages = append(s.messages, conversation.Event{
			Role: conversation.Assistant,
			Name: "未完成",
			Text: s.streamText,
		})
	}
	s.streamText = ""
	s.progressText = ""
	s.active = nil

	// 3. 设置最终状态。此时后台任务已不再请求模型或执行工具。
	switch {
	case errors.Is(runErr, context.Canceled):
		s.status = "已停止"
		s.messages = append(s.messages, conversation.Event{
			Role: conversation.System,
			Text: "任务已停止。",
		})
	case runErr != nil:
		s.status = "失败"
		s.messages = append(s.messages, conversation.Event{
			Role: conversation.Error,
			Text: runErr.Error(),
		})
	default:
		s.status = "已完成"
	}

	// 4. 提醒有新结果并通知本项等待者；退出时还要等整个 Session.Run 结束。
	s.hasUnreadResult = true
	s.updatedAt = time.Now()
	job.err = runErr
	close(job.done)
	return runErr
}
