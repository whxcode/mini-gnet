package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/net"
)

// TestChatToolRoundTrip 从应用入口验证请求模型、执行工具、回传结果和保存最终回答的完整顺序。
func TestChatToolRoundTrip(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"pwd-call","type":"function","function":{"name":"run_bash","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || last.ToolCallID != "pwd-call" || last.Content == "" {
				t.Errorf("missing tool result: %+v", last)
			}
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"已获取目录"},"finish_reason":"stop"}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := config.Config{ChatsDir: t.TempDir(), ProjectDir: t.TempDir()}
	session, err := startSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{config: cfg, session: session, client: &net.Client{BaseURL: server.URL, Model: "test", HTTP: server.Client()}}
	defer a.Close()
	if err := a.Chat(context.Background(), "获取当前目录"); err != nil {
		t.Fatal(err)
	}
	history := a.session.ToChatCompletionMessage()
	if requests.Load() != 2 || len(history) != 5 || history[4].Content != "已获取目录" {
		t.Fatalf("unexpected history: %+v", history)
	}
	if err := a.session.Err(); err != nil {
		t.Fatal(err)
	}
}

// TestChatCancellationJoinsWorker 验证应用取消后等待 Agent 退出，不遗留写会话的后台任务。
func TestChatCancellationJoinsWorker(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"部分回答\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	cfg := config.Config{ChatsDir: t.TempDir(), ProjectDir: t.TempDir()}
	session, err := startSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{config: cfg, session: session, client: &net.Client{BaseURL: server.URL, Model: "test", HTTP: server.Client()}}
	defer a.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Chat(ctx, "等待回答") }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("model did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Chat did not wait for worker shutdown correctly")
	}
	if len(a.session.ToChatCompletionMessage()) != 2 {
		t.Fatal("incomplete answer saved as complete")
	}
}
