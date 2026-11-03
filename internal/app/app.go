// Package app 负责输入、命令和会话切换；执行流程依次进入 session、agent 和 task 包。
package app

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/agent"
	commands "github.com/whxcode/mini-gnet/internal/command"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/tools/builtin"
)

// App 持有应用唯一的运行配置，并管理会话列表和输入循环。
type App struct {
	config         *config.Store       // 应用唯一的共享配置。
	CurrentSession *sessions.Session   // 当前查看和提交提问的会话。
	agent          *agent.Agent        // 所有会话共用的执行服务。
	registry       *tools.Registry     // App 和 Agent 共享同一份只读工具注册表。
	quit           bool                // /exit 的 handler 设置，主循环据此退出。
	opened         []*sessions.Session // 已打开的会话，切换后仍保留各自队列。
	closed         bool                // 防止重复关闭会话和历史文件。
}

// newApp 组装应用共用的配置和 Agent，不创建会话或启动协程。
func newApp(cfg config.Config, registry *tools.Registry) *App {
	store := config.NewStore(cfg)

	return &App{
		config:   store,
		registry: registry,
		agent:    agent.New(store, registry),
	}
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

	registry, err := builtin.NewRegistry()
	if err != nil {
		return nil, err
	}

	record, err := sessions.NewHistory(cfg)
	if err != nil {
		return nil, err
	}

	a := newApp(cfg, registry)
	a.AttachSession(record)

	return a, nil
}

// Close 先取消并等待所有任务，再关闭每个会话的记录文件；切换时不会关闭旧会话。
func (a *App) Close() error {
	if a.closed {
		return nil
	}

	a.closed = true

	for _, s := range a.opened {
		s.Shutdown()
	}

	var err error

	for _, s := range a.opened {
		err = errors.Join(err, s.Close())
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
		command, args, err := commands.Lookup(query)
		if err != nil {
			if fromEditor {
				a.CurrentSession.Draft.Reset()
			}

			a.reportError(err)
			continue
		}

		if command != nil {
			if fromEditor {
				a.CurrentSession.Draft.Reset()
			}

			if err := command.Handler(a, args); err != nil {
				a.reportError(err)
			}

			continue
		}

		// 3. 只提交到当前会话的队列；任务由该会话自己的 Run 循环顺序执行。
		session := a.CurrentSession
		_, err = session.Enqueue(ctx, query)
		if err != nil {
			a.reportError(err)
			continue // 保留输入草稿，方便修改或切换会话。
		}

		session.Draft.Reset()

	}

	return nil
}
