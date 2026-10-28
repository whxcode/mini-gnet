package net

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sashabaranov/go-openai"
)

// TestStreamReadsTextAndToolFragments 验证真实 SSE 协议的文字、分片工具参数和多个工具顺序。
func TestStreamReadsTextAndToolFragments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if !request.Stream || request.Model != "test-model" {
			t.Error("missing streaming request")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		frames := []string{
			`{"choices":[{"delta":{"content":"你"}}]}`,
			`{"choices":[{"delta":{"content":"好"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":2,"id":"second","type":"function","function":{"name":"run_python","arguments":"{\"code\":"}},{"index":0,"id":"first","type":"function","function":{"name":"run_bash","arguments":"{\"command\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"pwd\"}"}},{"index":2,"function":{"arguments":"\"print(2)\"}"}}]},"finish_reason":"tool_calls"}]}`,
		}
		for _, frame := range frames {
			fmt.Fprintf(w, "data: %s\n\n", frame)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Model: "test-model", HTTP: server.Client()}
	stream, err := client.OpenStream(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var text string
	for {
		chunk, err := stream.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		text += chunk
	}
	result := stream.Result()
	if text != "你好" || result.Text != text || result.Finish != openai.FinishReasonToolCalls {
		t.Fatalf("bad answer: %+v", result)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("missing tools: %+v", result.ToolCalls)
	}
	if result.ToolCalls[0].ID != "first" || result.ToolCalls[0].Function.Arguments != `{"command":"pwd"}` ||
		result.ToolCalls[1].ID != "second" || result.ToolCalls[1].Function.Arguments != `{"code":"print(2)"}` {
		t.Fatalf("incorrect tool assembly: %+v", result.ToolCalls)
	}
	result.ToolCalls[0].Function.Name = "changed"
	if stream.Result().ToolCalls[0].Function.Name != "run_bash" {
		t.Fatal("Result leaked mutable state")
	}
}

// TestStreamCancellationStopsRead 验证等待下一段输出时取消 context 可以中断网络读取。
func TestStreamCancellationStopsRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"部分回答\"}}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{BaseURL: server.URL, Model: "test", HTTP: server.Client()}
	stream, err := client.OpenStream(ctx, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	text, err := stream.Next()
	if err != nil || text != "部分回答" {
		t.Fatalf("first chunk: %q, %v", text, err)
	}
	cancel()
	if _, err := stream.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("read did not cancel: %v", err)
	}
}
