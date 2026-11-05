package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/tools/builtin"
	"github.com/whxcode/mini-gnet/views/message"
)

// TestProjectPromptAcrossRounds 验证恢复旧会话后替换系统规则、同任务快照稳定、下一任务重新读取。
func TestProjectPromptAcrossRounds(t *testing.T) {
	project := t.TempDir()
	skillDir := filepath.Join(project, ".agent", "skills", "demo")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	rulesPath := filepath.Join(project, ".agent", "AGENTS.md")
	if err := os.WriteFile(rulesPath, []byte("规则版本 A"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: demo\ndescription: 摘要验证标记\n---\n正文隔离验证标记"), 0o600); err != nil {
		t.Fatal(err)
	}

	requests := make(chan openai.ChatCompletionRequest, 3)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")

		if calls.Add(1) == 1 {
			// 模拟任务执行中修改文件：本项下一轮应仍然使用 A，下一项才使用 B。
			if err := os.WriteFile(rulesPath, []byte("规则版本 B"), 0o600); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"check-dir","type":"function","function":{"name":"run_bash","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	chatsDir := t.TempDir()
	original, err := message.New(chatsDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := original.AppendSystemMessage("旧会话系统提示词"); err != nil {
		original.Close()
		t.Fatal(err)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	history, err := message.Resume(chatsDir, original.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer history.Close()

	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(config.Config{
		BaseURL:    server.URL,
		Model:      "test",
		ProjectDir: project,
	})
	runner := New(store, registry, nil)
	for _, query := range []string{"查看目录", "再次提问"} {
		job := task.New(context.Background(), query)
		err := runner.Run(job, history, NewOutput(nil))
		job.Cancel()
		if err != nil {
			t.Fatal(err)
		}
	}

	for index, version := range []string{"规则版本 A", "规则版本 A", "规则版本 B"} {
		request := <-requests
		systems := 0
		for _, msg := range request.Messages {
			if msg.Role == openai.ChatMessageRoleSystem {
				systems++
			}
		}
		text := request.Messages[0].Content
		if systems != 1 || strings.Count(text, version) != 1 || strings.Count(text, "摘要验证标记") != 1 {
			t.Fatalf("第 %d 轮规则未正确合并，system 数量 %d", index+1, systems)
		}
		if strings.Contains(text, "旧会话系统提示词") || strings.Contains(text, "正文隔离验证标记") {
			t.Fatal("旧规则或技能正文被带入请求")
		}
	}

	before := len(history.Records())
	if err := os.WriteFile(skillPath, []byte("格式已损坏"), 0o600); err != nil {
		t.Fatal(err)
	}
	job := task.New(context.Background(), "不能发送")
	defer job.Cancel()
	if err := runner.Run(job, history, NewOutput(nil)); err == nil || !strings.Contains(err.Error(), skillPath) {
		t.Fatal("损坏技能应在发起请求前返回具体错误", err)
	}
	if calls.Load() != 3 || len(history.Records()) != before {
		t.Fatal("加载失败仍然请求了模型或修改了历史")
	}
}
