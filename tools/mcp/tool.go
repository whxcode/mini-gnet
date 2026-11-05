// Package mcp 将 MCP 远程工具适配为统一工具接口，不参与模型对话循环。
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	remote "github.com/whxcode/mini-gnet/internal/mcp"
	"github.com/whxcode/mini-gnet/tools"
)

// Tool 绑定模型名称、远程原名和本任务的连接，Agent 无需识别来源。
type Tool struct {
	definition tools.Definition
	client     *remote.Client
	remoteName string
}

// Alias 产生稳定、最多 64 字符的模型名称；摘要保留原服务与原名身份，避免截断冲突。
func Alias(server, name string) string {
	digest := sha256.Sum256([]byte(server + "\x00" + name))
	label := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			return r
		}

		return '_'
	}, server+"__"+name)
	if len(label) > 40 {
		label = label[:40]
	}

	return "mcp__" + label + "__" + hex.EncodeToString(digest[:8])
}

// Bind 将发现的工具绑定到连接，不修改原始 inputSchema 或伪造参数约束。
func Bind(server string, client *remote.Client, definitions []*sdk.Tool) ([]tools.Tool, error) {
	var implementations []tools.Tool
	for _, definition := range definitions {
		if definition == nil || definition.Name == "" {
			return nil, fmt.Errorf("MCP %s 返回空工具名称", server)
		}

		input, err := json.Marshal(definition.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("编码 MCP 工具 %s 的参数：%w", definition.Name, err)
		}

		implementations = append(implementations, &Tool{
			definition: tools.Definition{
				SchemaVersion: 1,
				Name:          Alias(server, definition.Name),
				Description:   "MCP " + server + " / " + definition.Name + "：" + definition.Description,
				InputSchema:   input,
				OutputSchema:  json.RawMessage(`{"type":"object","additionalProperties":true}`),
			},
			client:     client,
			remoteName: definition.Name,
		})
	}

	return implementations, nil
}

// Definition 返回统一声明，真实服务地址不混入模型参数。
func (t *Tool) Definition() tools.Definition {
	return t.definition.Clone()
}

// Execute 保留 MCP 内容与结构化结果；isError 不能被协议成功掩盖。
func (t *Tool) Execute(ctx context.Context, request tools.Request) (tools.Result, error) {
	result, err := t.client.CallTool(ctx, t.remoteName, request.Arguments)
	if err != nil {
		return tools.Result{}, err
	}

	if result.IsError {
		return tools.Result{Data: result}, fmt.Errorf("MCP 工具 %s 执行失败，详见返回内容", t.remoteName)
	}

	return tools.Result{Data: result}, nil
}

// Prepare 连接本次选中的服务，获取并组装任务工具表；失败时也关闭已建立的连接。
// 连接组由任务执行入口 defer Close，多轮请求始终使用同一份注册表。
func Prepare(
	ctx context.Context,
	base *tools.Registry,
	servers []remote.Server,
) (registry *tools.Registry, connections *Connections, err error) {
	connections = &Connections{}
	defer func() {
		if err != nil {
			err = errors.Join(err, connections.Close())
		}
	}()

	var implementations []tools.Tool
	for _, server := range servers {
		client, connectErr := remote.Connect(ctx, server)
		if connectErr != nil {
			return nil, connections, connectErr
		}

		connections.clients = append(connections.clients, client)

		details, listErr := client.Details(ctx)
		if listErr != nil {
			return nil, connections, listErr
		}

		bound, bindErr := Bind(server.Name, client, details.Tools)
		if bindErr != nil {
			return nil, connections, bindErr
		}

		implementations = append(implementations, bound...)
	}

	registry, err = base.WithRemote(implementations...)
	return registry, connections, err
}

// Connections 仅持有本次任务创建的连接，不在 Session 之间共享。
type Connections struct {
	clients []*remote.Client
}

// Close 逐项释放连接并合并错误，不丢失前面连接的关闭失败。
func (c *Connections) Close() error {
	var err error
	for _, client := range c.clients {
		err = errors.Join(err, client.Close())
	}

	return err
}
