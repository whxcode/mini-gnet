package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestClientDiscoveryAndCall 使用本地 HTTP 服务验证握手、JSON/SSE、分页和原始工具调用。
func TestClientDiscoveryAndCall(t *testing.T) {
	for _, sse := range []bool{false, true} {
		t.Run(fmt.Sprintf("SSE=%v", sse), func(t *testing.T) {
			var initialized atomic.Bool
			var listed atomic.Int32
			var closed atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					closed.Add(1)
					w.WriteHeader(http.StatusNoContent)
					return
				}
				var request struct {
					ID     any                        `json:"id"`
					Method string                     `json:"method"`
					Params map[string]json.RawMessage `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				var result any
				switch request.Method {
				case "initialize":
					w.Header().Set("Mcp-Session-Id", "test-session")
					result = map[string]any{
						"protocolVersion": "2025-06-18",
						"capabilities":    map[string]any{"tools": map[string]any{}},
						"serverInfo":      map[string]any{"name": "local-docs", "version": "1.0"},
						"instructions":    "本地测试服务",
					}
				case "notifications/initialized":
					initialized.Store(true)
					w.WriteHeader(http.StatusAccepted)
					return
				case "tools/list":
					if !initialized.Load() || r.Header.Get("Mcp-Session-Id") != "test-session" || r.Header.Get("Mcp-Protocol-Version") != "2025-06-18" {
						t.Error("初始化通知或协议会话头缺失")
					}
					listed.Add(1)
					cursor := ""
					if request.Params["cursor"] != nil {
						if err := json.Unmarshal(request.Params["cursor"], &cursor); err != nil {
							t.Error(err)
						}
					}
					if cursor == "" {
						result = map[string]any{"tools": []any{map[string]any{
							"name": "search", "description": "搜索", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"anyOf": []any{map[string]any{"type": "string"}, map[string]any{"type": "null"}}}}},
						}}, "nextCursor": "page2"}
					} else {
						if cursor != "page2" {
							t.Error("分页 cursor 错误")
						}
						result = map[string]any{"tools": []any{map[string]any{"name": "fetch", "inputSchema": map[string]any{"type": "object"}}}}
					}
				case "tools/call":
					if string(request.Params["name"]) != `"search"` || string(request.Params["arguments"]) != `{"query":"Go"}` {
						t.Errorf("工具原始参数丢失：%v", request.Params)
					}
					result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "found"}}, "structuredContent": map[string]any{"count": 1}, "isError": true}
				default:
					t.Errorf("未知请求 %s", request.Method)
					w.WriteHeader(400)
					return
				}
				body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
				if err != nil {
					t.Error(err)
					return
				}
				if sse {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "event: message\ndata: %s\n\n", body)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.Write(body)
				}
			}))
			defer server.Close()
			cfg := Server{Name: "docs", Transport: StreamableHTTP, URL: server.URL}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, err := Connect(ctx, cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { client.Close() })
			details, err := client.Details(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if details.ServerName != "local-docs" || details.Protocol != "2025-06-18" || len(details.Tools) != 2 || listed.Load() != 2 {
				t.Fatalf("发现结果错误：%+v", details)
			}
			result, err := client.CallTool(ctx, "search", json.RawMessage(`{"query":"Go"}`))
			if err != nil || !result.IsError || len(result.Content) != 1 || result.StructuredContent == nil {
				t.Fatal("结果内容或业务错误丢失", err)
			}
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			if closed.Load() != 1 {
				t.Fatal("未关闭远程会话")
			}
		})
	}
}

// TestConnectCancellation 验证上游取消会中止等待，不依赖 HTTP 默认超时。
func TestConnectCancellation(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(204)
			return
		}
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			w.WriteHeader(400)
			return
		}
		if request.Method != "initialize" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		once.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	defer server.Close()
	defer close(released)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Connect(ctx, Server{Name: "slow", Transport: StreamableHTTP, URL: server.URL})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("请求未启动")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("取消原因丢失：%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("取消后仍在等待")
	}
}

// TestMicrosoftLearnDiscovery 是显式启用的真实服务检查，默认测试不访问公网。
func TestMicrosoftLearnDiscovery(t *testing.T) {
	if os.Getenv("MINI_GNET_LIVE_MCP") != "1" {
		t.Skip("设置 MINI_GNET_LIVE_MCP=1 才请求微软公共 MCP")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	details, err := Inspect(ctx, Server{
		Name:      "microsoft-learn",
		Transport: StreamableHTTP,
		URL:       "https://learn.microsoft.com/api/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Tools) == 0 {
		t.Fatal("微软服务没有返回工具")
	}
	t.Logf("服务=%s 版本=%s 协议=%s 工具数=%d", details.ServerName, details.Version, details.Protocol, len(details.Tools))
	for _, tool := range details.Tools {
		t.Log(tool.Name)
	}
}
