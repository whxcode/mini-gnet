package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/views/message"
)

// TestFinishReasonControlsCompletion 验证正常 stop 才产生最终回答，EOF、截断和异常原因不能冒充完成。
func TestFinishReasonControlsCompletion(t *testing.T) {
	for _, reason := range []string{"stop", "length", "content_filter", "", "tool_calls", "unknown"} {
		t.Run(reason, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				data, err := json.Marshal(map[string]any{
					"choices": []any{
						map[string]any{
							"delta":         map[string]string{"content": "模型文字"},
							"finish_reason": reason,
						},
					},
				})
				if err != nil {
					t.Error(err)

					return
				}

				fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			}))
			defer server.Close()

			history, err := message.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}

			defer history.Close()

			runner := New(config.NewStore(config.Config{
				BaseURL: server.URL,
				Model:   "test",
			}), nil, nil)
			output := NewOutput(nil)
			job := task.New(context.Background(), "问题")
			defer job.Cancel()

			err = runner.Run(job, history, output)
			if (err == nil) != (reason == "stop") {
				t.Fatalf("finish_reason=%q, err=%v", reason, err)
			}

			output.Finish()
			finals := 0

			for _, event := range output.Snapshot().Messages {
				if event.Stage == Final {
					finals++
				}
			}

			if reason == "stop" {
				if finals != 1 || len(history.Records()) != 2 || !history.Records()[1].Response.Final {
					t.Fatal("stop 没有保存唯一最终回答")
				}
			} else if finals != 0 || len(history.Records()) != 1 {
				t.Fatal("异常结束被保存为完整回答")
			}
		})
	}
}
