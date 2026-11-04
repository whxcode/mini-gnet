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

	client := &Client{
		BaseURL: server.URL,
		Model:   "test-model",
		HTTP:    server.Client(),
	}
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

	client := &Client{
		BaseURL: server.URL,
		Model:   "test",
		HTTP:    server.Client(),
	}
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

// TestStreamUsage 验证末尾独立用量帧、同帧上报、重复累计值和未上报时的区别。
func TestStreamUsage(t *testing.T) {
	cases := []struct {
		name   string
		frames []string
		known  bool
	}{
		{
			name: "usage-only-final-frame",
			frames: []string{
				`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}]}`,
				`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":25,"total_tokens":125}}`,
			},
			known: true,
		},
		{
			name: "repeated-cumulative-usage",
			frames: []string{
				`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":25,"total_tokens":125}}`,
				`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":25,"total_tokens":125}}`,
			},
			known: true,
		},
		{
			name: "provider-does-not-report",
			frames: []string{
				`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}],"usage":null}`,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request openai.ChatCompletionRequest
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
				}
				if request.StreamOptions == nil || !request.StreamOptions.IncludeUsage {
					t.Error("没有请求流式 usage")
				}

				w.Header().Set("Content-Type", "text/event-stream")
				for _, frame := range tc.frames {
					fmt.Fprintf(w, "data: %s\n\n", frame)
				}
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()

			client := Client{BaseURL: server.URL, Model: "test"}
			stream, err := client.OpenStream(context.Background(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()

			for {
				_, err := stream.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}

			result := stream.Result()
			if result.Text != "完成" || (result.Usage != nil) != tc.known {
				t.Fatalf("用量状态或正文不正确：%+v", result)
			}
			if tc.known {
				if result.Usage.PromptTokens != 100 || result.Usage.CompletionTokens != 25 || result.Usage.TotalTokens != 125 {
					t.Fatalf("重复计数或漏计：%+v", result.Usage)
				}
				result.Usage.TotalTokens = 999
				if stream.Result().Usage.TotalTokens != 125 {
					t.Fatal("返回值修改了流内部统计")
				}
			}
		})
	}
}
