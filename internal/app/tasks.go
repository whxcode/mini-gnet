package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// task 表示一次提问的执行，而 Session 是可反复提问的整个聊天会话。
// 主循环先创建 task，再用 go session.RunTask(job) 执行；这些配置在执行期间不再修改。
type task struct {
	ctx        context.Context    // 本次执行的取消信号，模型请求和工具调用都使用它。
	cancel     context.CancelFunc // /stop 调用它，请求本次执行停止，不影响其他会话。
	client     chatClient         // 启动时复制的模型客户端，避免执行中修改配置影响本任务。
	tools      []openai.Tool      // 本次请求允许模型调用的工具声明。
	projectDir string             // 本次工具操作使用的工作目录。
	done       chan struct{}      // 任务结束后关闭；退出程序时等待它，再关闭历史文件。
	err        error              // 最终执行错误；其他协程必须等 done 关闭后才能读取。
}

// prepareTask 由主循环同步调用：检查能否执行、保存用户问题、准备取消信号并标记忙碌。
// 它不请求模型，也不启动 goroutine；必须先准备成功，再调用 RunTask。
func (s *Session) prepareTask(parent context.Context, query string, tools []openai.Tool) (*task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 同一会话还在执行时拒绝新问题，防止两次回答混进同一份上下文。
	if s.running {
		return nil, fmt.Errorf("当前会话正在执行任务，请 /new 新建会话，或 /stop 停止当前任务")
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("问题不能为空")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}

	// 2. 先保存问题。保存失败就返回，不请求模型。
	s.history.AppendUserMessage(query)
	if err := s.history.Err(); err != nil {
		return nil, err
	}

	// 3. 为这一次执行准备独立的取消信号、完成通知和配置副本。
	ctx, cancel := context.WithCancel(parent)
	client := s.client
	job := &task{
		ctx:        ctx,
		cancel:     cancel,
		client:     &client,
		tools:      append([]openai.Tool(nil), tools...),
		projectDir: s.config.ProjectDir,
		done:       make(chan struct{}),
	}

	// 4. 在启动后台任务之前标记忙碌，界面马上就能知道本会话正在执行。
	s.task = job
	s.running = true
	s.status = "执行中"
	s.streamText = ""
	s.progressText = "正在请求模型…"
	s.hasUnreadResult = false
	s.updatedAt = time.Now()

	if s.title == "新会话" {
		s.title = strings.Join(strings.Fields(query), " ")
	}
	s.messages = append(s.messages, conversation.Event{
		Role: conversation.User,
		Name: conversation.Username(),
		Text: query,
	})
	return job, nil
}

// finishTask 由 RunTask 退出时调用：保存最终状态，保留中断的文字，并通知等待者任务已结束。
// 所有内存状态和历史写入都结束后才关闭 done，避免程序退出时提前关闭文件。
func (s *Session) finishTask(job *task, runErr error) error {
	defer job.cancel() // 释放本次 context 的资源，正常完成也需要清理。

	// 1. 以实际取消状态和记录写入错误判断本轮是否成功。
	if job.ctx.Err() != nil {
		runErr = job.ctx.Err()
	}
	runErr = errors.Join(runErr, s.history.Err())

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
	s.running = false

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

	// 4. 提醒有新结果，并通知 App.Close 可以关闭本会话的历史文件。
	s.hasUnreadResult = true
	s.updatedAt = time.Now()
	job.err = runErr
	close(job.done)
	return runErr
}

// stop 由 /stop 调用，只发出停止请求；它不等待网络返回，界面仍可继续编辑或切换。
func (s *Session) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		s.status = "正在停止"
		s.task.cancel()
	}
}

// currentTask 由 App.Close 调用，安全取得最近一次任务，随后等待它的 done 信号。
func (s *Session) currentTask() *task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.task
}
