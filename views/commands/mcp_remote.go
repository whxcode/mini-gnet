package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/whxcode/mini-gnet/internal/mcp"
)

// mcpDetailsMsg 只携带一次发现结果；序号防止旧请求覆盖新打开的详情。
type mcpDetailsMsg struct {
	generation int
	details    *mcp.Details
	err        error
}

// beginDetails 启动可取消的详情加载，网络调用在 Tea Cmd 中执行，不阻塞按键。
func (m *mcpModel) beginDetails() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}

	m.requestContext, m.cancel = context.WithTimeout(m.ctx, 30*time.Second)
	m.generation++
	m.loading = true
	m.remote = nil
	m.remoteError = ""
	m.offset = 0
	return m.requestDetails()
}

// requestDetails 捕获本次选择和上下文，工作完成后仅返回消息，不修改界面状态。
func (m mcpModel) requestDetails() tea.Cmd {
	items := m.matches()
	if len(items) == 0 {
		return nil
	}

	server := items[m.cursor]
	ctx := m.requestContext
	generation := m.generation
	cancel := m.cancel
	return func() tea.Msg {
		defer cancel()
		details, err := mcp.Inspect(ctx, server)
		return mcpDetailsMsg{generation: generation, details: details, err: err}
	}
}

// remoteText 展示真实发现状态和完整工具 schema；远端说明只作显示，不作为系统指令执行。
func (m mcpModel) remoteText(server mcp.Server) string {
	lines := []string{
		"服务：" + server.Name,
		"简介：" + server.Description,
		"地址：" + server.URL,
		"配置：" + m.path,
		"",
	}

	switch {
	case m.loading:
		lines = append(lines, "正在连接并获取工具列表…")
	case m.remoteError != "":
		lines = append(lines, "获取失败："+m.remoteError, "按 r 重试，Esc 返回。")
	case m.remote != nil:
		lines = append(lines,
			"已获取远程信息",
			"服务端："+m.remote.ServerName+" "+m.remote.Version,
			"协议版本："+m.remote.Protocol,
			"服务说明："+m.remote.Instructions,
			fmt.Sprintf("工具：%d 个", len(m.remote.Tools)),
		)
		for _, tool := range m.remote.Tools {
			if tool == nil {
				continue
			}

			lines = append(lines, "", "◆ "+tool.Name, tool.Description, "输入参数：", schemaText(tool.InputSchema))
			if tool.OutputSchema != nil {
				lines = append(lines, "输出约束：", schemaText(tool.OutputSchema))
			}
		}
	default:
		lines = append(lines, "尚未获取远程信息，按 r 加载。")
	}

	return strings.Join(lines, "\n")
}

// schemaText 将服务原始 schema 排版为 JSON，不修改字段与约束。
func schemaText(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "无法显示 schema：" + err.Error()
	}

	return string(data)
}
