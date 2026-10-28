package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/net"
	toolsList "github.com/whxcode/mini-gnet/tools/list"
	"github.com/whxcode/mini-gnet/tools/run"
)

func TestPythonTool(t *testing.T) {
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

func (m *memoryMessages) AppendSystemMessage(s string) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "system", Content: s})
}
func (m *memoryMessages) AppendUserMessage(s string) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "user", Content: s})
}
func (m *memoryMessages) AppendAssistant(s string, calls []openai.ToolCall) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "assistant", Content: s, ToolCalls: calls})
}
func (m *memoryMessages) AppendTool(id, s string) {
	m.messages = append(m.messages, openai.ChatCompletionMessage{Role: "tool", ToolCallID: id, Content: s})
}
func (m *memoryMessages) ToChatCompletionMessage() []openai.ChatCompletionMessage {
	return append([]openai.ChatCompletionMessage(nil), m.messages...)
}

type scriptedClient struct {
	t        *testing.T
	answers  []net.StreamResult
	requests [][]openai.ChatCompletionMessage
}

func (c *scriptedClient) Stream(_ context.Context, messages []openai.ChatCompletionMessage, _ []openai.Tool, _ func(string) error) (net.StreamResult, error) {
	c.t.Helper()
	index := len(c.requests)
	if index >= len(c.answers) {
		c.t.Fatal("unexpected extra model request")
	}
	c.requests = append(c.requests, messages)
	return c.answers[index], nil
}

func pythonCall(code string) net.StreamResult {
	args, _ := json.Marshal(map[string]string{"code": code})
	return net.StreamResult{ToolCalls: []openai.ToolCall{{
		ID: "python-call", Type: "function",
		Function: openai.FunctionCall{Name: "run_python", Arguments: string(args)},
	}}}
}

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
	if err := runAgentTurn(context.Background(), c, m, nil, t.TempDir()); err != nil {
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
			err := runAgentTurn(context.Background(), c, &memoryMessages{}, nil, t.TempDir())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAgentOrdinaryAnswerDoesNotRetry(t *testing.T) {
	c := &scriptedClient{t: t, answers: []net.StreamResult{{Text: "Python 是一种编程语言。"}}}
	if err := runAgentTurn(context.Background(), c, &memoryMessages{}, nil, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
