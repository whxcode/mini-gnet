// Package app 按顺序组织应用流程。先读 app.go，再读 agent.go 即可理解一轮对话。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/net"
	toolsList "github.com/whxcode/mini-gnet/tools/list"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/input"
	"github.com/whxcode/mini-gnet/views/message"
)

// App 持有当前配置和会话，切换时直接更新字段，不向函数传递二级指针。
type App struct {
	config  config.Config
	client  *net.Client
	session *message.TempMessage
	tools   []openai.Tool
	input   *input.Reader
	quit    bool // /exit 的 handler 设置，主循环据此退出。
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
	return &App{
		config: cfg, tools: tools, session: session, input: input.NewReader(),
		client: &net.Client{BaseURL: cfg.BaseURL, ApiKey: cfg.APIKey, Model: cfg.Model, HTTP: &http.Client{}},
	}, nil
}

// Close 关闭退出时的当前会话；之前被替换的会话已在切换时关闭。
func (a *App) Close() error { return a.session.Close() }

// Run 是程序的主线：读输入 → 处理本地命令或执行对话 → 回到输入。
func (a *App) Run(ctx context.Context) error {
	conversation.Print(conversation.Event{Role: conversation.System, Text: fmt.Sprintf(
		"pwd: %s\nbase: %s\nmodel: %s\nsession: %s", a.config.ProjectDir, a.config.BaseURL, a.config.Model, a.session.SessionID)})
	for !a.quit {
		if err := ctx.Err(); err != nil {
			return err
		}
		query, err := a.input.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		if strings.TrimSpace(query) == "" {
			continue
		}

		// 主循环拿到 handler 后直接传入 App；命令的修改操作在 handler 内完成。
		command, args, err := lookupCommand(query)
		if err != nil {
			conversation.Print(conversation.Event{Role: conversation.Error, Text: err.Error()})
			continue
		}
		if command != nil {
			if err := command.Handler(a, args); err != nil {
				conversation.Print(conversation.Event{Role: conversation.Error, Text: err.Error()})
			}
			continue
		}

		err = a.Chat(ctx, query)
		if errors.Is(err, context.Canceled) {
			conversation.Print(conversation.Event{Role: conversation.System, Text: "已停止当前任务，可以继续输入。"})
		} else if err != nil {
			conversation.Print(conversation.Event{Role: conversation.Error, Text: err.Error()})
		}
		if err := a.session.Err(); err != nil {
			return fmt.Errorf("保存聊天记录失败：%w", err)
		}
	}
	return nil
}

// Chat 保存提问，然后直接启动 Agent 和显示界面。这是业务与界面之间唯一的并发边界。
// Agent 按顺序执行，界面同时监听输出和取消；界面退出后必须等 Agent 结束才能接受新问题。
func (a *App) Chat(parent context.Context, query string) error {
	a.session.AppendUserMessage(query)
	if err := a.session.Err(); err != nil {
		return err
	}
	conversation.Print(conversation.Event{Role: conversation.User, Name: conversation.Username(), Text: query})
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	events := make(chan conversation.Event, 64)
	done := make(chan error, 1)

	// 由应用直接运行 Agent；不再把业务函数交给 Bubble Tea 回调执行。
	go func() {
		err := runAgentTurn(ctx, a.client, a.session, a.tools, a.config.ProjectDir, events)
		close(events) // 先发送完所有显示消息，再通知界面本轮结束。
		done <- err
	}()
	viewErr := conversation.Run(ctx, cancel, events)
	cancel()           // 界面异常退出时也要停止网络请求和工具执行。
	agentErr := <-done // sendEvent 在取消后不会阻塞；等待本轮彻底结束再返回。
	if viewErr != nil {
		return viewErr
	}
	return agentErr
}
