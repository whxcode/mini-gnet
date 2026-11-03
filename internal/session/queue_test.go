package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	tasks "github.com/whxcode/mini-gnet/internal/task"
)

// TestQueuePreservesConversationOrder 验证排队提问不会泄漏到前一任务的工具轮次，且按 A/B/C 顺序执行。
func TestQueuePreservesConversationOrder(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)

			return
		}

		last := request.Messages[len(request.Messages)-1]
		wantModel := "initial-model"

		if last.Role == "user" && last.Content != "A" {
			wantModel = "new-for-waiting-tasks"
		}

		if request.Model != wantModel {
			t.Errorf("执行时读取配置错误：got %s, want %s", request.Model, wantModel)
		}

		w.Header().Set("Content-Type", "text/event-stream")

		if last.Role == "user" && last.Content == "A" {
			started <- "A"

			select {
			case <-release:
			case <-r.Context().Done():
				return
			}

			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"pwd","type":"function","function":{"name":"run_bash","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n\n")

			return
		}

		if last.Role == "tool" {
			for _, message := range request.Messages {
				if message.Role == "user" && message.Content != "A" {
					t.Error("A 的工具轮次提前读到了排队提问")
				}
			}

			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"answer-A"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")

			return
		}

		started <- last.Content
		fmt.Fprintf(
			w,
			"data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
			"answer-"+last.Content,
		)
	}))
	t.Cleanup(server.Close)
	cfg, s := sessionForTest(t, server.URL)
	first, err := s.Enqueue(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}

	awaitRequest(t, started, "A")
	second, err := s.Enqueue(context.Background(), "B")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	canceled, err := s.Enqueue(ctx, "不应执行")
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	cancel()
	third, err := s.Enqueue(context.Background(), "C")
	if err != nil {
		t.Fatal(err)
	}

	state := s.Snapshot()

	if strings.Join(state.Pending, ",") != "B,C" {
		t.Fatalf("等待顺序错误：%v", state.Pending)
	}

	if len(s.History.ToChatCompletionMessage()) != 2 {
		t.Fatal("排队时提前写入历史")
	}

	{
		next := cfg.Current()
		next.Model = "new-for-waiting-tasks"
		cfg.Set(next)
	}
	close(release)

	for _, job := range []*tasks.Task{first, second, third} {
		if err := waitTask(t, job); err != nil {
			t.Fatal(err)
		}
	}

	if err := waitTask(t, canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("消费循环没有跳过已取消的等待项：%v", err)
	}

	awaitRequest(t, started, "B")
	awaitRequest(t, started, "C")
	var dialogue []string

	for _, message := range s.History.ToChatCompletionMessage() {
		if message.Role == "user" || message.Role == "assistant" && len(message.ToolCalls) == 0 {
			dialogue = append(dialogue, message.Content)
		}
	}

	if strings.Join(dialogue, ",") != "A,answer-A,B,answer-B,C,answer-C" {
		t.Fatalf("对话顺序错误：%v", dialogue)
	}
}

// TestQueueCapacityAndClose 验证队列满时立即返回，退出会取消所有等待项，关闭消费循环并拒绝新提交。
func TestQueueCapacityAndClose(t *testing.T) {
	started := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		started <- "running"
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	_, s := sessionForTest(t, server.URL)
	active, err := s.Enqueue(context.Background(), "active")
	if err != nil {
		t.Fatal(err)
	}

	awaitRequest(t, started, "running")
	jobs := []*tasks.Task{active}

	for i := 0; i < 16; i++ {
		job, err := s.Enqueue(context.Background(), fmt.Sprintf("queued-%d", i))
		if err != nil {
			t.Fatal(err)
		}

		jobs = append(jobs, job)
	}

	full := make(chan error, 1)
	go func() {
		_, err := s.Enqueue(context.Background(), "overflow")
		full <- err
	}()

	select {
	case err := <-full:
		if err == nil || !strings.Contains(err.Error(), "已满") {
			t.Fatalf("队列满错误：%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("队列满导致主循环阻塞")
	}

	s.Shutdown()

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	for _, job := range jobs {
		if err := waitTask(t, job); !errors.Is(err, context.Canceled) {
			t.Fatalf("任务未取消：%v", err)
		}
	}

	if _, err := s.Enqueue(context.Background(), "after close"); err == nil {
		t.Fatal("关闭后仍能入队")
	}

	if len(s.History.ToChatCompletionMessage()) != 2 {
		t.Fatal("退出时执行了等待任务")
	}
}

// TestStopDoesNotLoseDequeuedTask 验证连续提交并立即停止时，已出队但未开始的任务也能结束。
func TestStopDoesNotLoseDequeuedTask(t *testing.T) {
	_, s := sessionForTest(t, "")

	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		job, err := s.Enqueue(ctx, "cancel")
		if err != nil {
			cancel()
			t.Fatal(err)
		}

		cancel()
		s.Stop()

		if err := waitTask(t, job); err == nil {
			t.Fatal("取消任务未返回错误")
		}
	}
}
