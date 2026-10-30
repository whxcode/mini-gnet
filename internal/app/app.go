// Package app 按顺序组织应用流程。先读 app.go，再读 agent.go 即可理解一轮对话。
package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	toolsList "github.com/whxcode/mini-gnet/tools/list"
)

// App 持有应用唯一的运行配置，并管理会话列表和输入循环。
type App struct {
	config         config.Config // 全部会话共用，只有主循环中的 /config 修改它。
	CurrentSession *Session
	tools          []openai.Tool
	quit           bool // /exit 的 handler 设置，主循环据此退出。
	opened         []*Session
	closed         bool
}

// New 按顺序加载配置、工具和会话；初始化成功后，调用者负责 Close。
func New() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.ProjectDir == "" {
		cfg.ProjectDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	tools, err := toolsList.GetToolsList()
	if err != nil {
		return nil, err
	}
	session, err := startSession(cfg)
	if err != nil {
		return nil, err
	}
	a := &App{
		config: cfg,
		tools:  tools,
	}
	a.attachSession(session)
	return a, nil
}

// Close 先取消并等待所有任务，再关闭每个会话的记录文件；切换时不会关闭旧会话。
func (a *App) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	for _, s := range a.opened {
		s.mu.Lock()
		s.closed = true
		close(s.taskQueue) // 关闭后不能再入队，Run 也不会启动尚未执行的任务。
		s.mu.Unlock()
		s.stop()
	}
	var err error
	for _, s := range a.opened {
		<-s.workerDone
		err = errors.Join(err, s.history.Close())
	}
	return err
}

// Run 专用于交互终端：读输入、执行命令或提交队列；管道入口独立放在 pipe.go。
func (a *App) Run(ctx context.Context) error {
	for !a.quit {
		if err := ctx.Err(); err != nil {
			return err
		}
		// 1. 读取输入。界面只交出文本，不判断命令或启动任务。
		query, fromEditor, err := a.readInput(ctx)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(query) == "" {
			continue
		}

		// 2. 命令直接交给 handler，不能写入聊天历史或发送给模型。
		command, args, err := lookupCommand(query)
		if err != nil {
			if fromEditor {
				a.CurrentSession.draft.Reset()
			}
			a.reportError(err)
			continue
		}
		if command != nil {
			if fromEditor {
				a.CurrentSession.draft.Reset()
			}
			if err := command.Handler(a, args); err != nil {
				a.reportError(err)
			}
			continue
		}
		// 3. 只提交到当前会话的队列；任务由该会话自己的 Run 循环顺序执行。
		session := a.CurrentSession
		_, err = a.enqueueTask(ctx, query)
		if err != nil {
			a.reportError(err)
			continue // 保留输入草稿，方便修改或切换会话。
		}

		session.draft.Reset()

	}
	return nil
}
