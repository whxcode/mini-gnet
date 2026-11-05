package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client 持有一次 MCP 连接；创建方在发现完成或任务结束时 Close。
// 协议初始化、JSON/SSE 响应和请求 ID 匹配交给官方 SDK。
type Client struct {
	session *sdk.ClientSession
}

// Details 是一次远程发现的结果，不写入 mcp.json，也不长期缓存工具列表。
type Details struct {
	ServerName   string
	Version      string
	Protocol     string
	Instructions string
	Tools        []*sdk.Tool
}

// Connect 校验地址并完成初始化；请求受上游取消和单次 HTTP 30 秒超时约束。
func Connect(ctx context.Context, server Server) (*Client, error) {
	server, err := Normalize(server)
	if err != nil {
		return nil, err
	}

	client := sdk.NewClient(&sdk.Implementation{
		Name:    "mini-gnet",
		Version: "0.1.0",
	}, &sdk.ClientOptions{
		Capabilities: &sdk.ClientCapabilities{},
	})
	transport := &sdk.StreamableClientTransport{
		Endpoint:             server.URL,
		HTTPClient:           &http.Client{Timeout: 30 * time.Second},
		DisableStandaloneSSE: true, // 当前按任务获取列表，不订阅服务器主动通知。
		MaxRetries:           -1,
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("连接 MCP %s：%w", server.Name, err)
	}

	return &Client{session: session}, nil
}

// Details 获取握手信息和全部工具，分页重复或超过上限时明确报错。
func (c *Client) Details(ctx context.Context) (*Details, error) {
	initialized := c.session.InitializeResult()
	details := &Details{
		Protocol:     initialized.ProtocolVersion,
		Instructions: initialized.Instructions,
	}

	if initialized.ServerInfo != nil {
		details.ServerName = initialized.ServerInfo.Name
		details.Version = initialized.ServerInfo.Version
	}

	if initialized.Capabilities == nil || initialized.Capabilities.Tools == nil {
		return details, nil
	}

	cursor := ""
	seen := make(map[string]bool)
	for page := 0; page < 100; page++ {
		result, err := c.session.ListTools(ctx, &sdk.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("获取 MCP 工具列表：%w", err)
		}

		details.Tools = append(details.Tools, result.Tools...)
		if result.NextCursor == "" {
			return details, nil
		}

		if seen[result.NextCursor] {
			return nil, fmt.Errorf("MCP 工具分页返回重复 cursor")
		}

		cursor = result.NextCursor
		seen[cursor] = true
	}

	return nil, fmt.Errorf("MCP 工具列表超过 100 页，停止获取")
}

// CallTool 转发远程原始名称和参数，协议错误与业务 isError 分别由 SDK 和适配器处理。
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (*sdk.CallToolResult, error) {
	return c.session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: arguments})
}

// Close 释放 SDK 会话；详情和任务各自拥有连接，取消不会影响其他任务。
func (c *Client) Close() error {
	return c.session.Close()
}

// Inspect 为详情页提供一次完整的发现流程，返回前关闭连接。
func Inspect(ctx context.Context, server Server) (details *Details, err error) {
	client, err := Connect(ctx, server)
	if err != nil {
		return nil, err
	}

	defer func() {
		err = errors.Join(err, client.Close())
	}()

	return client.Details(ctx)
}
