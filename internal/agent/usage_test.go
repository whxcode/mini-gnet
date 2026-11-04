package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/tools/builtin"
	"github.com/whxcode/mini-gnet/views/message"
)

// TestAgentCountsEveryModelRound 验证一个问题经过工具和最终回答两轮时均统计，工具不单独计费。
func TestAgentCountsEveryModelRound(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if strings.Contains(string(body["messages"]), "agent_response") {
			t.Error("统计元数据进入了模型消息")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		if calls == 1 {
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"one","type":"function","function":{"name":"run_bash","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}]}`+"\n\n")
		}
		fmt.Fprintf(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":%d,\"completion_tokens\":20,\"total_tokens\":%d}}\n\n", calls*100, calls*100+20)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	history, err := message.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer history.Close()

	store := config.NewStore(config.Config{
		BaseURL:       server.URL,
		Model:         "test",
		ProjectDir:    t.TempDir(),
		ContextWindow: 1000,
	})
	runner := New(store, registry)
	job := task.New(context.Background(), "查看当前目录")
	defer job.Cancel()
	if err := runner.Run(job, history, NewOutput(nil)); err != nil {
		t.Fatal(err)
	}

	u := history.Usage()
	if u.Reported != 2 || u.TotalTokens != 340 || u.Last == nil || u.Last.InputTokens != 200 || u.Last.ContextWindow != 1000 {
		t.Fatalf("多轮工具请求用量不正确：%+v", u)
	}
}
