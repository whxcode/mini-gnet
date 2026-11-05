package compact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
)

// TestGenerateValidatesResponse 验证历史仅作为数据、无工具注册，以及无效响应不能当作摘要。
func TestGenerateValidatesResponse(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"success", `{"choices":[{"delta":{"content":"目标：修复登录。待办：运行测试。"},"finish_reason":"stop"}]}`, ""},
		{"empty", `{"choices":[{"delta":{"content":"  "},"finish_reason":"stop"}]}`, "空摘要"},
		{"length", `{"choices":[{"delta":{"content":"未写完"},"finish_reason":"length"}]}`, "未完整结束"},
		{"missing_stop", `{"choices":[{"delta":{"content":"未写完"}}]}`, "未完整结束"},
		{"tool", `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"x","type":"function","function":{"name":"run_bash","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`, "未完整结束"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request openai.ChatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if len(request.Tools) != 0 || len(request.Messages) != 2 || request.Model != "test" {
					t.Errorf("错误的摘要请求：%+v", request)
				}
				if !strings.Contains(request.Messages[1].Content, "pending-call") || strings.Contains(request.Messages[1].Content, "OLD_SYSTEM") {
					t.Error("应保留中断工具调用作为数据，不沿用旧系统指令")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", tc.body)
			}))
			defer server.Close()

			history := []openai.ChatCompletionMessage{
				{Role: "system", Content: "OLD_SYSTEM"},
				{Role: "user", Content: "修复登录"},
				{Role: "assistant", ToolCalls: []openai.ToolCall{{ID: "pending-call", Type: "function", Function: openai.FunctionCall{Name: "run_bash", Arguments: `{}`}}}},
			}
			summary, err := Generate(context.Background(), config.Config{BaseURL: server.URL, Model: "test"}, history)
			if tc.want == "" {
				if err != nil || summary != "目标：修复登录。待办：运行测试。" {
					t.Fatalf("summary=%q err=%v", summary, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}

// TestGenerateCancellation 验证网络等待接受上游取消，不遗留摘要请求。
func TestGenerateCancellation(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"部分摘要\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := Generate(ctx, config.Config{BaseURL: server.URL}, []openai.ChatCompletionMessage{{Role: "user", Content: "原问题"}})
		result <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("模型请求未开始")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("未保留取消原因：%v", err)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("取消后服务端连接未关闭")
	}
}
