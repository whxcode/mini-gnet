package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

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

// agentTaskForTest 使用真实临时记录与预设响应，直接执行 Session.RunTask 而不经过界面。
func agentTaskForTest(t *testing.T, c *scriptedClient) (*Session, *task) {
	t.Helper()
	a := terminalTestApp(t)
	session := a.CurrentSession
	job, err := session.prepareTask(context.Background(), "测试提问", nil)
	if err != nil {
		t.Fatal(err)
	}
	job.client = c
	c.session = session
	return session, job
}

type scriptedClient struct {
	t        *testing.T
	answers  []net.StreamResult
	requests [][]openai.ChatCompletionMessage
	session  *Session
	streams  []*scriptedStream
	readErr  error
}

// OpenStream 用预设回答替代网络请求，并保存请求用于验证重试上下文。
func (c *scriptedClient) OpenStream(_ context.Context, messages []openai.ChatCompletionMessage, _ []openai.Tool) (net.ResponseStream, error) {
	c.t.Helper()
	index := len(c.requests)
	if index >= len(c.answers) {
		c.t.Fatal("unexpected extra model request")
	}
	c.requests = append(c.requests, messages)
	if index > 0 && !c.streams[index-1].closed {
		c.t.Fatal("上轮响应流未释放就开启下一轮请求")
	}
	stream := &scriptedStream{answer: c.answers[index], text: []rune(c.answers[index].Text), session: c.session, t: c.t, readErr: c.readErr}
	c.streams = append(c.streams, stream)
	return stream, nil
}

type scriptedStream struct {
	answer  net.StreamResult
	text    []rune
	index   int
	closed  bool
	session *Session
	t       *testing.T
	readErr error
}

// Next 每次提供一个字符，验证显示发生在完整回答之前。
func (s *scriptedStream) Next() (string, error) {
	// 再读取下一个字符前，上一个字符必须已经出现在会话快照中。
	if got := s.session.snapshot().StreamText; got != string(s.text[:s.index]) {
		s.t.Fatalf("流式片段没有及时更新会话：%q", got)
	}
	if s.index == len(s.text) {
		if s.readErr != nil {
			return "", s.readErr
		}
		return "", io.EOF
	}
	text := string(s.text[s.index])
	s.index++
	return text, nil
}

// Result 返回测试预设的完整回答和工具调用。
func (s *scriptedStream) Result() net.StreamResult { return s.answer }

// Close 记录资源释放，供失败和取消测试检查。
func (s *scriptedStream) Close() error { s.closed = true; return nil }

// TestAgentStreamsBeforeFinalMessage 验证文字先逐段显示，随后只提交一次完整回答。
func TestAgentStreamsBeforeFinalMessage(t *testing.T) {
	c := &scriptedClient{t: t, answers: []net.StreamResult{{Text: "你好，世界"}}}
	session, job := agentTaskForTest(t, c)
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
	if finalCount != 1 || session.snapshot().StreamText != "" || !c.streams[0].closed {
		t.Fatal("回答重复、流式状态未清空，或响应未关闭")
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
	c := &scriptedClient{t: t, answers: []net.StreamResult{
		pythonCall("print(os.cpu_count())"),
		{Text: "缺少 import os，请你自己运行。"},
		pythonCall("import os\nprint(os.cpu_count())"),
		{Text: "已获取 CPU 数量。"},
	}}
	session, job := agentTaskForTest(t, c)
	if err := session.RunTask(job); err != nil {
		t.Fatal(err)
	}
	if len(c.requests) != 4 {
		t.Fatalf("got %d model requests", len(c.requests))
	}
	for _, index := range []int{1, 2} {
		r := c.requests[index]
		if r[len(r)-1].Role != "system" || !strings.Contains(r[len(r)-1].Content, "NameError") {
			t.Fatal("missing actual failure and retry instruction")
		}
	}
	last := c.requests[3]
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
			c := &scriptedClient{t: t, answers: tc.answers}
			session, job := agentTaskForTest(t, c)
			err := session.RunTask(job)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestAgentOrdinaryAnswerDoesNotRetry 验证普通回答结束后不会无故继续请求模型。
func TestAgentOrdinaryAnswerDoesNotRetry(t *testing.T) {
	c := &scriptedClient{t: t, answers: []net.StreamResult{{Text: "Python 是一种编程语言。"}}}
	session, job := agentTaskForTest(t, c)
	if err := session.RunTask(job); err != nil {
		t.Fatal(err)
	}
}

// TestSessionCanContinueAfterStreamFailure 验证中途断流会关闭响应、保留未完成显示，且不阻止同一会话的下一任务。
func TestSessionCanContinueAfterStreamFailure(t *testing.T) {
	failure := errors.New("连接中断")
	c := &scriptedClient{t: t, readErr: failure, answers: []net.StreamResult{{Text: "部分回答"}, {Text: "第二次完成"}}}
	session, first := agentTaskForTest(t, c)
	if err := session.RunTask(first); !errors.Is(err, failure) {
		t.Fatalf("未返回流错误：%v", err)
	}
	if err := waitTask(t, first); !errors.Is(err, failure) {
		t.Fatalf("失败任务没有正确通知结束：%v", err)
	}
	state := session.snapshot()
	if state.Running || state.Status != "失败" || !c.streams[0].closed {
		t.Fatal("流失败后仍在执行，或连接没有关闭")
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
	c.readErr = nil
	second, err := session.prepareTask(context.Background(), "继续", nil)
	if err != nil {
		t.Fatal(err)
	}
	second.client = c
	if err := session.RunTask(second); err != nil {
		t.Fatal(err)
	}
	if first == second || session.snapshot().Status != "已完成" || !c.streams[1].closed {
		t.Fatal("下一轮任务未独立完成")
	}
}
