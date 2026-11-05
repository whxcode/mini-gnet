package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/views/message"
)

// TestRunWithoutSession 验证核心对话可以独立调用，不需要 App、Session、终端或输出回调。
func TestRunWithoutSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"独立运行成功\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	history, err := message.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	defer history.Close()

	store := config.NewStore(config.Config{
		BaseURL:    server.URL,
		Model:      "test",
		ProjectDir: t.TempDir(),
	})
	runner := New(store, nil, nil)
	output := NewOutput(nil)
	job := task.New(context.Background(), "你好")
	defer job.Cancel()

	if err := runner.Run(job, history, output); err != nil {
		t.Fatal(err)
	}

	messages := history.ToChatCompletionMessage()

	if len(messages) != 2 || messages[1].Content != "独立运行成功" {
		t.Fatal("核心对话未保存结果")
	}

	if snapshot := output.Snapshot(); len(snapshot.Messages) != 2 || snapshot.StreamText != "" {
		t.Fatal("输出记录不完整")
	}

	select {
	case <-job.Done:
		t.Fatal("Agent 不应接管队列的任务完成通知")
	default:
	}
}
