package app

import (
	"context"
	"encoding/json"
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

type memoryMessages struct {
	messages []openai.ChatCompletionMessage
}

// AppendSystemMessage 在测试会话中追加系统消息。
func (m *memoryMessages) AppendSystemMessage(s string) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "system", Content: s})
}

// AppendUserMessage 在测试会话中追加用户提问。
func (m *memoryMessages) AppendUserMessage(s string) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "user", Content: s})
}

// AppendAssistant 在测试会话中保存回答和工具调用。
func (m *memoryMessages) AppendAssistant(s string, calls []openai.ToolCall) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "assistant", Content: s, ToolCalls: calls})
}

// AppendTool 在测试会话中关联工具调用 ID 与执行结果。
func (m *memoryMessages) AppendTool(id, s string) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "tool", ToolCallID: id, Content: s})
}

// ToChatCompletionMessage 返回测试会话的消息副本，避免临时重试提示污染原历史。
func (m *memoryMessages) ToChatCompletionMessage() []openai.ChatCompletionMessage {
	return append([]openai.ChatCompletionMessage(nil), m.messages...)
}

type scriptedClient struct {
	t        *testing.T
	answers  []net.StreamResult
	requests [][]openai.ChatCompletionMessage
}

// OpenStream 用预设回答替代网络请求，并保存请求用于验证重试上下文。
func (c *scriptedClient) OpenStream(_ context.Context, messages []openai.ChatCompletionMessage, _ []openai.Tool) (net.ResponseStream, error) {
	c.t.Helper()
	index := len(c.requests)
	if index >= len(c.answers) {
		c.t.Fatal("unexpected extra model request")
	}
	c.requests = append(c.requests, messages)
	return &scriptedStream{answer: c.answers[index], text: []rune(c.answers[index].Text)}, nil
}

type scriptedStream struct {
	answer net.StreamResult
	text   []rune
	index  int
	closed bool
}

// Next 每次提供一个字符，验证显示发生在完整回答之前。
func (s *scriptedStream) Next() (string, error) {
	if s.index == len(s.text) {
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
	events := make(chan conversation.Event, 64)
	err := runAgentTurn(context.Background(), c, &memoryMessages{}, nil, t.TempDir(), events)
	close(events)
	if err != nil {
		t.Fatal(err)
	}
	var chunks string
	finalCount := 0
	for e := range events {
		if e.Kind == conversation.Delta {
			if finalCount != 0 {
				t.Fatal("完整回答出现后才收到流式片段")
			}
			chunks += e.Text
		}
		if e.Kind == conversation.Message && e.Role == conversation.Assistant {
			finalCount++
			if e.Text != chunks {
				t.Fatalf("流式文本与完整回答不一致: %q / %q", chunks, e.Text)
			}
		}
	}
	if finalCount != 1 || chunks != "你好，世界" {
		t.Fatalf("回答重复或流式输出丢失: final=%d, chunks=%q", finalCount, chunks)
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
	m := &memoryMessages{}
	m.AppendUserMessage("使用临时 Python 获取系统信息")
	if err := runAgentTurn(context.Background(), c, m, nil, t.TempDir(), nil); err != nil {
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
			err := runAgentTurn(context.Background(), c, &memoryMessages{}, nil, t.TempDir(), nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestAgentOrdinaryAnswerDoesNotRetry 验证普通回答结束后不会无故继续请求模型。
func TestAgentOrdinaryAnswerDoesNotRetry(t *testing.T) {
	c := &scriptedClient{t: t, answers: []net.StreamResult{{Text: "Python 是一种编程语言。"}}}
	if err := runAgentTurn(context.Background(), c, &memoryMessages{}, nil, t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
}
