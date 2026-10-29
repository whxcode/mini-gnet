// Package app 按顺序组织应用流程。先读 app.go，再读 agent.go 即可理解一轮对话。
package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/net"
	toolsList "github.com/whxcode/mini-gnet/tools/list"
)

// App 管理会话列表和输入循环；记录、配置、客户端都只属于 Session，不在 App 重复保存。
type App struct {
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
	current := newSession(session, cfg, net.Client{BaseURL: cfg.BaseURL, ApiKey: cfg.APIKey, Model: cfg.Model, HTTP: &http.Client{}})
	return &App{
		CurrentSession: current,
		opened:         []*Session{current},
		tools:          tools,
	}, nil
}

// Close 先取消并等待所有任务，再关闭每个会话的记录文件；切换时不会关闭旧会话。
func (a *App) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	for _, s := range a.opened {
		s.stop()
	}
	var err error
	for _, s := range a.opened {
		if task := s.currentTask(); task != nil {
			<-task.done
		}
		err = errors.Join(err, s.history.Close())
	}
	return err
}

// Run 专用于交互终端：读输入、执行命令、启动后台任务；管道入口独立放在 pipe.go。
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
		// 3. 在启动 goroutine 前占用本会话、保存提问、捕获配置。
		// 如果放到 goroutine 里才标记忙碌，快速连续发送可能启动两个任务。
		session := a.CurrentSession
		job, err := session.prepareTask(ctx, query, a.tools)
		if err != nil {
			a.reportError(err)
			continue // 保留输入草稿，方便修改或切换会话。
		}
		session.draft.Reset()

		// 4. 后台执行任务，主循环随即继续接收输入。
		go session.RunTask(job)
	}
	return nil
}
