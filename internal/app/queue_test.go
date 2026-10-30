package app

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
		if request.Model != "initial-model" {
			t.Error("排队任务没有保留提交时的模型配置")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if last.Role == "user" && last.Content == "A" {
			started <- "A"
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"pwd","type":"function","function":{"name":"run_bash","arguments":"{\"command\":\"pwd\"}"}}]}}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		if last.Role == "tool" {
			for _, message := range request.Messages {
				if message.Role == "user" && message.Content != "A" {
					t.Error("A 的工具轮次提前读到了排队提问")
				}
			}
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"answer-A"}}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		started <- last.Content
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\ndata: [DONE]\n\n", "answer-"+last.Content)
	}))
	t.Cleanup(server.Close)
	a := taskTestApp(t, server)
	first, err := a.enqueueTask(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "A")
	second, err := a.enqueueTask(context.Background(), "B")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	canceled, err := a.enqueueTask(ctx, "不应执行")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	third, err := a.enqueueTask(context.Background(), "C")
	if err != nil {
		t.Fatal(err)
	}
	state := a.CurrentSession.snapshot()
	if strings.Join(state.Pending, ",") != "B,C" {
		t.Fatalf("等待顺序错误：%v", state.Pending)
	}
	if len(a.CurrentSession.history.ToChatCompletionMessage()) != 2 {
		t.Fatal("排队时提前写入历史")
	}
	a.config.Model = "only-for-future-submissions"
	close(release)
	for _, job := range []*task{first, second, third} {
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
	for _, message := range a.CurrentSession.history.ToChatCompletionMessage() {
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
	a := taskTestApp(t, server)
	active, err := a.enqueueTask(context.Background(), "active")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "running")
	jobs := []*task{active}
	for i := 0; i < 16; i++ {
		job, err := a.enqueueTask(context.Background(), fmt.Sprintf("queued-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, job)
	}
	full := make(chan error, 1)
	go func() { _, err := a.enqueueTask(context.Background(), "overflow"); full <- err }()
	select {
	case err := <-full:
		if err == nil || !strings.Contains(err.Error(), "已满") {
			t.Fatalf("队列满错误：%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("队列满导致主循环阻塞")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	for _, job := range jobs {
		if err := waitTask(t, job); !errors.Is(err, context.Canceled) {
			t.Fatalf("任务未取消：%v", err)
		}
	}
	select {
	case <-a.CurrentSession.workerDone:
	default:
		t.Fatal("消费协程未退出")
	}
	if _, err := a.enqueueTask(context.Background(), "after close"); err == nil {
		t.Fatal("关闭后仍能入队")
	}
	if len(a.CurrentSession.history.ToChatCompletionMessage()) != 2 {
		t.Fatal("退出时执行了等待任务")
	}
}

// TestStopDoesNotLoseDequeuedTask 验证连续提交并立即停止时，已出队但未开始的任务也能结束。
func TestStopDoesNotLoseDequeuedTask(t *testing.T) {
	a := terminalTestApp(t)
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		job, err := a.enqueueTask(ctx, "cancel")
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
		a.CurrentSession.stop()
		if err := waitTask(t, job); err == nil {
			t.Fatal("取消任务未返回错误")
		}
	}
}
