package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/internal/mcp"
	"github.com/whxcode/mini-gnet/internal/query"
	mcptools "github.com/whxcode/mini-gnet/tools/mcp"
)

// TestMCPTaskScopeAndHistory 验证多轮工具调用、业务失败恢复、历史记录，以及下一任务撤销工具权限。
func TestMCPTaskScopeAndHistory(t *testing.T) {
	var calls atomic.Int32
	var connections atomic.Int32
	var closed atomic.Int32
	service := sdk.NewServer(&sdk.Implementation{Name: "test-docs", Version: "1"}, nil)
	service.AddTool(&sdk.Tool{
		Name:        "search",
		Description: "查询文档",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"anyOf":[{"type":"string"},{"type":"null"}]}},"required":["query"],"additionalProperties":false}`),
	}, func(ctx context.Context, request *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		if string(request.Params.Arguments) != `{"query":"Go"}` {
			t.Errorf("远端参数错误：%s", request.Params.Arguments)
		}
		failed := calls.Add(1) == 1
		return &sdk.CallToolResult{
			Content:           []sdk.Content{&sdk.TextContent{Text: "实际工具返回内容"}},
			StructuredContent: map[string]any{"source": "local-docs"},
			IsError:           failed,
		}, nil
	})
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return service }, &sdk.StreamableHTTPOptions{JSONResponse: true})
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			closed.Add(1)
		}
		if r.Method == http.MethodPost && r.Header.Get("Mcp-Session-Id") == "" {
			connections.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	defer remote.Close()

	alias := mcptools.Alias("docs", "search")
	var modelCalls atomic.Int32
	var mu sync.Mutex
	var requests []openai.ChatCompletionRequest
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		requests = append(requests, request)
		mu.Unlock()

		step := modelCalls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if step == 1 || step == 2 || step == 4 {
			call := map[string]any{
				"choices": []any{map[string]any{
					"delta": map[string]any{"tool_calls": []any{map[string]any{
						"index": 0, "id": fmt.Sprintf("call-%d", step), "type": "function",
						"function": map[string]any{"name": alias, "arguments": `{"query":"Go"}`},
					}}},
					"finish_reason": "tool_calls",
				}},
			}
			body, err := json.Marshal(call)
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", body)
		} else {
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer model.Close()
	a := skillTestApp(t, model.URL)
	if err := a.MCPManager().Add(mcp.Server{Name: "docs", Transport: mcp.StreamableHTTP, URL: remote.URL}); err != nil {
		t.Fatal(err)
	}
	baseCount := len(a.registry.ModelTools())
	first, err := a.CurrentSession.Enqueue(context.Background(), "请使用 /mcp{docs} 和 /mcp{docs} 查询 Go")
	if err != nil {
		t.Fatal(err)
	}
	if err := waitTask(t, first); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || connections.Load() != 1 || closed.Load() != 1 {
		t.Fatalf("本次多轮没有复用并关闭连接：call=%d connect=%d close=%d", calls.Load(), connections.Load(), closed.Load())
	}

	second, err := a.CurrentSession.Enqueue(context.Background(), "普通问题")
	if err != nil {
		t.Fatal(err)
	}
	if err := waitTask(t, second); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || connections.Load() != 1 {
		t.Fatal("下一任务仍能调用上次 MCP")
	}
	if len(a.registry.ModelTools()) != baseCount {
		t.Fatal("任务改变了共享工具集合")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 5 {
		t.Fatalf("模型轮次错误：%d", len(requests))
	}
	for i, request := range requests {
		found := false
		for _, tool := range request.Tools {
			if tool.Function.Name == alias {
				found = true
				data, err := json.Marshal(tool.Function.Parameters)
				if err != nil || !strings.Contains(string(data), "anyOf") {
					t.Fatal("远程 schema 被简化或丢失")
				}
			}
		}
		if found != (i < 3) {
			t.Fatalf("第 %d 轮工具范围错误", i+1)
		}
	}
	toolMessage := requests[1].Messages[len(requests[1].Messages)-2] // 失败后的末尾有补充纠错提示。
	if toolMessage.Role != "tool" || toolMessage.ToolCallID != "call-1" || !strings.Contains(toolMessage.Content, `"ok":false`) || !strings.Contains(toolMessage.Content, "实际工具返回内容") {
		t.Fatalf("业务失败或历史关联丢失：%+v", toolMessage)
	}
	foundDenied := false
	for _, msg := range requests[4].Messages {
		if msg.ToolCallID == "call-4" && strings.Contains(msg.Content, "TOOL_NOT_FOUND") {
			foundDenied = true
		}
	}
	if !foundDenied {
		t.Fatal("过期 MCP 工具调用未被拒绝")
	}
}

// TestMCPCompletionAndUnknownReference 验证斜线候选确认只插入标签，未知引用不会发给模型。
func TestMCPCompletionAndUnknownReference(t *testing.T) {
	a := terminalTestApp(t)
	if err := a.MCPManager().Add(mcp.Server{Name: "docs", Description: "文档服务", Transport: mcp.StreamableHTTP, URL: "https://example.invalid/mcp"}); err != nil {
		t.Fatal(err)
	}
	a.CurrentSession.Draft.SetValue("请使用 /mcp{do")
	m := newTerminal(a, context.Background())
	m.refreshCompletions()
	if len(m.completions) != 1 || !m.completions[0].mcp {
		t.Fatalf("MCP 候选错误：%+v", m.completions)
	}
	if !m.completionKey(tea.KeyPressMsg{Code: tea.KeyEnter}) {
		t.Fatal("未消费候选确认按键")
	}
	if m.done || a.CurrentSession.Draft.Value() != "请使用 /mcp{docs}" {
		t.Fatal("候选确认应只插入标签")
	}
	parts, err := a.CurrentSession.Draft.Submission()
	if err != nil || len(parts.MCPNames()) != 1 || parts[1].Kind != query.MCP {
		t.Fatal("结构化 MCP 引用丢失", err)
	}
	job, err := a.CurrentSession.Enqueue(context.Background(), "/mcp{missing} 查询")
	if err != nil {
		t.Fatal(err)
	}
	if err := waitTask(t, job); err == nil || !strings.Contains(err.Error(), "未配置 MCP") {
		t.Fatal("未知 MCP 未在请求模型前报错", err)
	}
}
