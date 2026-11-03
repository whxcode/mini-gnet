package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/internal/agent"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	tasks "github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/net"
	"github.com/whxcode/mini-gnet/views/conversation"
	"github.com/whxcode/mini-gnet/views/message"
)

// agentTaskForTest 通过公开会话接口运行 Agent，使用本地 HTTP 服务提供预设回答。
func agentTaskForTest(t *testing.T, c *scriptedModel) (*sessions.Session, *tasks.Task) {
	t.Helper()
	cfg, s := sessionForTest(t, "")
	c.session = s
	server := httptest.NewServer(http.HandlerFunc(c.serveHTTP))
	t.Cleanup(server.Close)
	next := cfg.Current()
	next.BaseURL = server.URL
	cfg.Set(next)
	job, err := s.Enqueue(context.Background(), "测试提问")
	if err != nil {
		t.Fatal(err)
	}

	return s, job
}

// scriptedModel 是测试用 HTTP 服务，按顺序发送预设回答，并记录真实请求。
type scriptedModel struct {
	t            *testing.T                       // 报告模拟服务的协议错误。
	mu           sync.Mutex                       // 保护请求记录和失败开关。
	answers      []net.StreamResult               // 每轮预设的模型响应。
	requests     [][]openai.ChatCompletionMessage // 收到的上下文。
	session      *sessions.Session                // 检查片段是否已显示。
	failResponse bool                             // 用损坏的数据帧模拟流中断。
}

// serveHTTP 逐字发送 SSE，确认片段已显示后再发送下一字，验证真实网络流的增量更新。
func (c *scriptedModel) serveHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var request openai.ChatCompletionRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		c.t.Error(err)
		http.Error(w, "invalid request", http.StatusBadRequest)

		return
	}

	index := len(c.requests)

	if index >= len(c.answers) {
		c.t.Error("unexpected extra model request")
		http.Error(w, "unexpected request", http.StatusInternalServerError)

		return
	}

	c.requests = append(c.requests, request.Messages)
	answer := c.answers[index]
	w.Header().Set("Content-Type", "text/event-stream")
	text := []rune(answer.Text)

	for i, char := range text {
		data, err := json.Marshal(map[string]any{
			"choices": []any{
				map[string]any{
					"delta": map[string]any{"content": string(char)},
				},
			},
		})
		if err != nil {
			c.t.Error(err)

			return
		}

		fmt.Fprintf(w, "data: %s\n\n", data)
		w.(http.Flusher).Flush()

		// 下一片段发送前，上一片段必须已进入显示快照；超时明确失败，避免测试挂住。
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		ticker := time.NewTicker(time.Millisecond)

		for c.session.Snapshot().StreamText != string(text[:i+1]) {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				c.t.Error("流式片段未及时显示")
				ticker.Stop()
				cancel()

				return
			}
		}

		ticker.Stop()
		cancel()
	}

	if c.failResponse {
		fmt.Fprint(w, "data: invalid-json\n\n")

		return
	}

	finish := answer.Finish

	if finish == "" {
		finish = openai.FinishReasonStop

		if len(answer.ToolCalls) > 0 {
			finish = openai.FinishReasonToolCalls
		}
	}

	data, err := json.Marshal(map[string]any{
		"choices": []any{
			map[string]any{
				"delta":         map[string]any{"tool_calls": answer.ToolCalls},
				"finish_reason": finish,
			},
		},
	})
	if err != nil {
		c.t.Error(err)

		return
	}

	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
}

// requestHistory 返回已收到请求的副本，与服务协程同步后供断言读取。
func (c *scriptedModel) requestHistory() [][]openai.ChatCompletionMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([][]openai.ChatCompletionMessage(nil), c.requests...)
}

// TestAgentStreamsBeforeFinalMessage 验证文字先逐段显示，随后只提交一次完整回答。
func TestAgentStreamsBeforeFinalMessage(t *testing.T) {
	c := &scriptedModel{
		t: t,
		answers: []net.StreamResult{
			{Text: "你好，世界"},
		},
	}
	session, job := agentTaskForTest(t, c)

	if err := waitTask(t, job); err != nil {
		t.Fatal(err)
	}

	finalCount := 0

	for _, e := range session.Snapshot().Messages {
		if e.Kind == conversation.Message && e.Role == conversation.Assistant {
			finalCount++

			if e.Text != "你好，世界" {
				t.Fatalf("完整回答不一致: %q", e.Text)
			}
		}
	}

	if finalCount != 1 || session.Snapshot().StreamText != "" {
		t.Fatal("回答重复或流式状态未清空")
	}
}

// pythonCall 构造模型请求运行 Python 的假回答。
func pythonCall(code string) net.StreamResult {
	args, _ := json.Marshal(map[string]string{"code": code})

	return net.StreamResult{
		ToolCalls: []openai.ToolCall{
			{
				ID:   "python-call",
				Type: "function",
				Function: openai.FunctionCall{
					Name:      "run_python",
					Arguments: string(args),
				},
			},
		},
	}
}

// TestAgentRepairsPython 验证工具失败后继续交给模型修复，并在成功后清除重试提示。
func TestAgentRepairsPython(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}

	c := &scriptedModel{
		t: t,
		answers: []net.StreamResult{
			pythonCall("print(os.cpu_count())"),
			pythonCall("import os\nprint(os.cpu_count())"),
			{Text: "已获取 CPU 数量。"},
		},
	}
	_, job := agentTaskForTest(t, c)

	if err := waitTask(t, job); err != nil {
		t.Fatal(err)
	}

	if len(c.requestHistory()) != 3 {
		t.Fatalf("got %d model requests", len(c.requestHistory()))
	}

	for _, index := range []int{1} {
		r := c.requestHistory()[index]

		if r[len(r)-1].Role != "system" || !strings.Contains(r[len(r)-1].Content, "NameError") {
			t.Fatal("missing actual failure and retry instruction")
		}
	}

	last := c.requestHistory()[2]

	if last[len(last)-1].Role != "tool" || !strings.Contains(last[len(last)-1].Content, `"exit_code":0`) {
		t.Fatal("successful retry was not fed back to model, or stale reminder remained")
	}
}

// TestAgentRetryLimits 验证连续工具失败仍受重试上限约束。
func TestAgentRetryLimits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []net.StreamResult
		want    string
	}{
		{
			"execution-failures",
			[]net.StreamResult{pythonCall(""), pythonCall(""), pythonCall("")},
			"执行失败 3 次",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &scriptedModel{
				t:       t,
				answers: tc.answers,
			}
			_, job := agentTaskForTest(t, c)
			err := waitTask(t, job)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestAgentOrdinaryAnswerDoesNotRetry 验证普通回答结束后不会无故继续请求模型。
func TestAgentOrdinaryAnswerDoesNotRetry(t *testing.T) {
	c := &scriptedModel{
		t: t,
		answers: []net.StreamResult{
			{Text: "Python 是一种编程语言。"},
		},
	}
	_, job := agentTaskForTest(t, c)

	if err := waitTask(t, job); err != nil {
		t.Fatal(err)
	}
}

// TestStopEndsAfterToolFailure 验证即使工具失败，模型 stop 后仍按协议结束，不评判回答内容。
func TestStopEndsAfterToolFailure(t *testing.T) {
	c := &scriptedModel{
		t: t,
		answers: []net.StreamResult{
			pythonCall(""),
			{Text: "请自行运行"},
		},
	}
	s, job := agentTaskForTest(t, c)

	if err := waitTask(t, job); err != nil {
		t.Fatal(err)
	}

	if len(c.requestHistory()) != 2 {
		t.Fatal("stop 后仍在请求模型")
	}

	last := s.Snapshot().Messages[len(s.Snapshot().Messages)-1]

	if last.Stage != agent.Final || last.Text != "请自行运行" {
		t.Fatal("没有按协议显示最终回答")
	}
}

// TestMultipleRoundsHaveOneFinal 验证一个提问经历两次工具调用后，仅最后的 stop 回答独立显示，恢复后分层不变。
func TestMultipleRoundsHaveOneFinal(t *testing.T) {
	call := net.StreamResult{
		Text: "先检查目录",
		ToolCalls: []openai.ToolCall{
			{
				ID:   "pwd-call",
				Type: "function",
				Function: openai.FunctionCall{
					Name:      "run_bash",
					Arguments: `{"command":"pwd"}`,
				},
			},
		},
	}
	c := &scriptedModel{
		t: t,
		answers: []net.StreamResult{
			call,
			call,
			{Text: "最后的结论"},
		},
	}
	s, job := agentTaskForTest(t, c)

	if err := waitTask(t, job); err != nil {
		t.Fatal(err)
	}

	users := 0
	process := 0
	finals := 0

	for _, event := range s.Snapshot().Messages {
		if event.Role == agent.User {
			users++
		}

		if event.Stage == agent.Process {
			process++
		}

		if event.Stage == agent.Final {
			finals++
		}
	}

	if users != 1 || process != 2 || finals != 1 || len(c.requestHistory()) != 3 {
		t.Fatalf("分层错误：user=%d process=%d final=%d", users, process, finals)
	}

	path := s.History.Path()
	id := s.History.SessionID
	s.Shutdown()

	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := message.Resume(filepath.Dir(path), id)
	if err != nil {
		t.Fatal(err)
	}

	defer restored.Close()

	// 恢复显示不需要启动后台任务。
	reopened := sessions.New(restored, nil)
	got := reopened.Snapshot().Messages

	if got[1].Stage != agent.Process || got[len(got)-1].Stage != agent.Final {
		t.Fatal("恢复后丢失中间轮次或最终回答标记")
	}
}

// TestSessionCanContinueAfterStreamFailure 验证中途断流会保留未完成显示，且不阻止同一会话的下一任务。
func TestSessionCanContinueAfterStreamFailure(t *testing.T) {
	c := &scriptedModel{
		t:            t,
		failResponse: true,
		answers: []net.StreamResult{
			{Text: "部分回答"},
			{Text: "第二次完成"},
		},
	}
	session, first := agentTaskForTest(t, c)

	if err := waitTask(t, first); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("未返回流错误：%v", err)
	}

	if err := waitTask(t, first); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("失败任务没有正确通知结束：%v", err)
	}

	state := session.Snapshot()

	if state.Running || state.Status != "失败" {
		t.Fatal("流失败后仍在执行")
	}

	foundPartial := false

	for _, event := range state.Messages {
		if event.Name == "未完成" && event.Text == "部分回答" {
			foundPartial = true
		}
	}

	if !foundPartial {
		t.Fatal("中断前已收到的文字丢失")
	}

	for _, message := range session.History.ToChatCompletionMessage() {
		if message.Role == "assistant" {
			t.Fatal("部分回答被保存为完整历史")
		}
	}

	c.mu.Lock()
	c.failResponse = false
	c.mu.Unlock()
	second, err := session.Enqueue(context.Background(), "继续")
	if err != nil {
		t.Fatal(err)
	}

	if err := waitTask(t, second); err != nil {
		t.Fatal(err)
	}

	if first == second || session.Snapshot().Status != "已完成" {
		t.Fatal("下一轮任务未独立完成")
	}
}
