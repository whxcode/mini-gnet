package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/net"
	toolsList "github.com/whxcode/mini-gnet/tools/list"
	"github.com/whxcode/mini-gnet/tools/run"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// TestPythonTool 验证 Python 工具注册及成功、失败结果的协议格式。
func TestPythonTool(t *testing.T) {
	t.Chdir("../..") // 工具声明仍位于项目根目录。
	tools, err := toolsList.GetToolsList()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools {
		if tool.Function != nil && tool.Function.Name == "run_python" {
			found = true
		}
	}
	if !found {
		t.Fatal("run_python is not registered")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	for _, code := range []string{
		"import platform\nprint(platform.system())\n",
		"raise RuntimeError('tool failure')\n",
	} {
		args, _ := json.Marshal(map[string]string{"code": code})
		output := executeTool(context.Background(), t.TempDir(), "run_python", string(args))
		var result struct {
			run.RunResult
			Error string `json:"error"`
		}
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			t.Fatalf("invalid tool response %q: %v", output, err)
		}
		if strings.Contains(code, "raise") {
			if result.Error == "" || result.ExitCode == 0 || !strings.Contains(result.Stderr, "tool failure") {
				t.Fatalf("missing failure details: %+v", result)
			}
		} else if result.Error != "" || result.ExitCode != 0 || strings.TrimSpace(result.Stdout) == "" {
			t.Fatalf("system information failed: %+v", result)
		} else {
			t.Logf("system information returned: %s", result.Stdout)
		}
	}
}

// agentTaskForTest 用本地 HTTP 模型运行真实请求流程，不向 Session 或 task 注入客户端。
func agentTaskForTest(t *testing.T, c *scriptedModel) (*App, *Session, *task) {
	t.Helper()
	a := terminalTestApp(t)
	session := a.CurrentSession
	c.session = session
	server := httptest.NewServer(http.HandlerFunc(c.serveHTTP))
	t.Cleanup(server.Close)
	a.config.BaseURL = server.URL
	a.config.Model = "test"
	job, err := a.prepareTask(context.Background(), "测试提问")
	if err != nil {
		t.Fatal(err)
	}
	return a, session, job
}

// scriptedModel 是测试用 HTTP 服务，按顺序发送预设回答，并记录真实请求。
type scriptedModel struct {
	t            *testing.T                       // 报告模拟服务的协议错误。
	mu           sync.Mutex                       // 保护请求记录和失败开关。
	answers      []net.StreamResult               // 每轮预设的模型响应。
	requests     [][]openai.ChatCompletionMessage // 收到的上下文。
	session      *Session                         // 检查片段是否已显示。
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
		data, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": string(char)}}}})
		if err != nil {
			c.t.Error(err)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		w.(http.Flusher).Flush()
		// 下一片段发送前，上一片段必须已进入显示快照；超时明确失败，避免测试挂住。
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		ticker := time.NewTicker(time.Millisecond)
		for c.session.snapshot().StreamText != string(text[:i+1]) {
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
	data, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": answer.ToolCalls}, "finish_reason": "stop"}}})
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
	c := &scriptedModel{t: t, answers: []net.StreamResult{{Text: "你好，世界"}}}
	_, session, job := agentTaskForTest(t, c)
	if err := session.RunTask(job); err != nil {
		t.Fatal(err)
	}
	finalCount := 0
	for _, e := range session.snapshot().Messages {
		if e.Kind == conversation.Message && e.Role == conversation.Assistant {
			finalCount++
			if e.Text != "你好，世界" {
				t.Fatalf("完整回答不一致: %q", e.Text)
			}
		}
	}
	if finalCount != 1 || session.snapshot().StreamText != "" {
		t.Fatal("回答重复或流式状态未清空")
	}
}

// pythonCall 构造模型请求运行 Python 的假回答。
func pythonCall(code string) net.StreamResult {
	args, _ := json.Marshal(map[string]string{"code": code})
	return net.StreamResult{ToolCalls: []openai.ToolCall{{
		ID: "python-call", Type: "function",
		Function: openai.FunctionCall{Name: "run_python", Arguments: string(args)},
	}}}
}

// TestAgentRepairsPythonAfterTextOnlyReply 验证 Python 失败后模型仅解释错误时仍继续修复，并在成功后清除重试提示。
func TestAgentRepairsPythonAfterTextOnlyReply(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	c := &scriptedModel{t: t, answers: []net.StreamResult{
		pythonCall("print(os.cpu_count())"),
		{Text: "缺少 import os，请你自己运行。"},
		pythonCall("import os\nprint(os.cpu_count())"),
		{Text: "已获取 CPU 数量。"},
	}}
	_, session, job := agentTaskForTest(t, c)
	if err := session.RunTask(job); err != nil {
		t.Fatal(err)
	}
	if len(c.requestHistory()) != 4 {
		t.Fatalf("got %d model requests", len(c.requestHistory()))
	}
	for _, index := range []int{1, 2} {
		r := c.requestHistory()[index]
		if r[len(r)-1].Role != "system" || !strings.Contains(r[len(r)-1].Content, "NameError") {
			t.Fatal("missing actual failure and retry instruction")
		}
	}
	last := c.requestHistory()[3]
	if last[len(last)-1].Role != "tool" || !strings.Contains(last[len(last)-1].Content, `"exit_code":0`) {
		t.Fatal("successful retry was not fed back to model, or stale reminder remained")
	}
}

// TestAgentRetryLimits 验证文字提醒和执行失败都有明确的停止次数。
func TestAgentRetryLimits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []net.StreamResult
		want    string
	}{
		{"text-only", []net.StreamResult{pythonCall(""), {Text: "请自行运行"}, {Text: "请自行运行"}, {Text: "请自行运行"}}, "模型仍未调用工具"},
		{"execution-failures", []net.StreamResult{pythonCall(""), pythonCall(""), pythonCall("")}, "执行失败 3 次"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &scriptedModel{t: t, answers: tc.answers}
			_, session, job := agentTaskForTest(t, c)
			err := session.RunTask(job)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestAgentOrdinaryAnswerDoesNotRetry 验证普通回答结束后不会无故继续请求模型。
func TestAgentOrdinaryAnswerDoesNotRetry(t *testing.T) {
	c := &scriptedModel{t: t, answers: []net.StreamResult{{Text: "Python 是一种编程语言。"}}}
	_, session, job := agentTaskForTest(t, c)
	if err := session.RunTask(job); err != nil {
		t.Fatal(err)
	}
}

// TestSessionCanContinueAfterStreamFailure 验证中途断流会保留未完成显示，且不阻止同一会话的下一任务。
func TestSessionCanContinueAfterStreamFailure(t *testing.T) {
	c := &scriptedModel{t: t, failResponse: true, answers: []net.StreamResult{{Text: "部分回答"}, {Text: "第二次完成"}}}
	a, session, first := agentTaskForTest(t, c)
	if err := session.RunTask(first); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("未返回流错误：%v", err)
	}
	if err := waitTask(t, first); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("失败任务没有正确通知结束：%v", err)
	}
	state := session.snapshot()
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
	for _, message := range session.history.ToChatCompletionMessage() {
		if message.Role == "assistant" {
			t.Fatal("部分回答被保存为完整历史")
		}
	}
	c.mu.Lock()
	c.failResponse = false
	c.mu.Unlock()
	second, err := a.prepareTask(context.Background(), "继续")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.RunTask(second); err != nil {
		t.Fatal(err)
	}
	if first == second || session.snapshot().Status != "已完成" {
		t.Fatal("下一轮任务未独立完成")
	}
}
