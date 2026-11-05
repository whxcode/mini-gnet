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
	"github.com/whxcode/mini-gnet/internal/mcp"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/tools/builtin"
	"github.com/whxcode/mini-gnet/tools/external"
)

// App 持有应用唯一的运行配置，并管理会话列表和输入循环。
type App struct {
	mcpManager     *mcp.Manager        // 共享服务配置，任务开始时读取快照。
	config         *config.Store       // 应用唯一的共享配置。
	CurrentSession *sessions.Session   // 当前查看和提交提问的会话。
	agent          *agent.Agent        // 所有会话共用的执行服务。
	registry       *tools.Registry     // App 和 Agent 共享同一份只读工具注册表。
	quit           bool                // /exit 的 handler 设置，主循环据此退出。
	opened         []*sessions.Session // 已打开的会话，切换后仍保留各自队列。
	closed         bool                // 防止重复关闭会话和历史文件。
}

// newApp 加载 MCP 配置并组装共享组件，不创建会话或启动协程。
func newApp(cfg config.Config, registry *tools.Registry) (*App, error) {
	manager, err := mcp.Open(cfg.MCPConfigPath)
	if err != nil {
		return nil, err
	}

	store := config.NewStore(cfg)

	return &App{
		config:     store,
		mcpManager: manager,
		registry:   registry,
		agent:      agent.New(store, registry, manager),
	}, nil
}

// MakeApp 按顺序加载配置、工具和会话；初始化成功后，调用者负责 Close。
func MakeApp() (*App, error) {
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

	implementations, err := builtin.Load()
	if err != nil {
		return nil, err
	}

	externalTools, err := external.LoadDir(cfg.ToolsDir)
	if err != nil {
		return nil, err
	}

	implementations = append(implementations, externalTools...)
	registry, err := tools.NewRegistry(implementations...)
	if err != nil {
		return nil, err
	}

	a, err := newApp(cfg, registry)
	if err != nil {
		return nil, err
	}

	record, err := sessions.NewHistory(cfg)
	if err != nil {
		return nil, err
	}

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

			if err := command.Handler(ctx, a, args); err != nil {
				a.reportError(err)
			}

			continue
		}

		// 3. 只提交到当前会话的队列；任务由该会话自己的 Run 循环顺序执行。
		session := a.CurrentSession
		input, err := session.Draft.Submission()
		if err != nil {
			a.reportError(err)
			continue
		}

		_, err = session.EnqueueInput(ctx, input)
		if err != nil {
			a.reportError(err)
			continue // 保留输入草稿，方便修改或切换会话。
		}

		session.Draft.Reset()

	}

	return nil
}
