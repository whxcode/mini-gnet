package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sashabaranov/go-openai"
	imp "github.com/whxcode/mini-gnet/interface"
	"github.com/whxcode/mini-gnet/net"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// chatClient 只要求模型能打开一个可主动读取的流，测试可以提供假客户端。
type chatClient interface {
	// OpenStream 返回可由 Agent 顺序读取的响应流。
	OpenStream(context.Context, []openai.ChatCompletionMessage, []openai.Tool) (net.ResponseStream, error)
}

// runAgentTurn 顺序执行一轮任务：请求模型、读取回答、保存消息、执行工具，再进入下一轮。
// Python 重试状态只属于当前任务，events 仅用于通知界面，不控制业务执行。
func runAgentTurn(ctx context.Context, client chatClient, msg imp.Message, tools []openai.Tool, projectDir string, events chan<- conversation.Event) error {
	pythonPending := false
	failures, reminders := 0, 0
	lastFailure := ""
	for step := 0; step < 8; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		messages := msg.ToChatCompletionMessage()
		if pythonPending {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: "上次 run_python 执行失败，用户请求尚未完成。请根据以下错误修正完整 Python 源码，立即再次调用 run_python。不能仅解释错误、展示代码或让用户手动执行。NameError 表示名称未定义，例如使用 os.cpu_count() 前必须实际加入 import os。只修复原任务，不扩大操作范围。\n实际错误：\n" + lastFailure,
			})
		}
		sendEvent(ctx, events, conversation.Event{Kind: conversation.Status, Role: conversation.Assistant, Text: "正在生成 · Ctrl+C 停止"})
		// 主动请求并读取回答，读完后再决定是否执行工具。
		answer, err := readAnswer(ctx, client, messages, tools, events)
		if err != nil {
			return err
		}
		msg.AppendAssistant(answer.Text, answer.ToolCalls)
		if strings.TrimSpace(answer.Text) != "" {
			sendEvent(ctx, events, conversation.Event{Role: conversation.Assistant, Text: answer.Text})
		}
		if len(answer.ToolCalls) == 0 {
			if pythonPending {
				if reminders >= 2 {
					return fmt.Errorf("Python 任务未完成：模型仍未调用工具修复错误。最后的执行结果：\n%s", lastFailure)
				}
				reminders++
				sendEvent(ctx, events, conversation.Event{Role: conversation.System, Text: "Python 尚未成功，要求模型继续修复并执行。"})
				continue
			}
			return nil
		}
		batchFailed := false
		pythonSucceeded := false
		for _, call := range answer.ToolCalls {
			sendEvent(ctx, events, conversation.Event{Kind: conversation.Status, Role: conversation.Tool, Name: call.Function.Name, Text: "正在执行 · Ctrl+C 停止"})
			sendEvent(ctx, events, conversation.Event{Role: conversation.Tool, Name: call.Function.Name + " · 调用", Text: toolInputText(call.Function.Name, call.Function.Arguments)})
			// 即使耗尽重试次数，也要为每个工具调用保存结果，保持协议完整。
			output := "Python 重试次数已用尽，本次工具调用未执行。"
			if failures < 3 {
				output = executeTool(ctx, projectDir, call.Function.Name, call.Function.Arguments)
			}
			msg.AppendTool(call.ID, output)
			sendEvent(ctx, events, conversation.Event{Role: conversation.Tool, Name: call.Function.Name + " · 结果", Text: toolOutputText(call.Function.Name, output)})
			if call.Function.Name != "run_python" {
				continue
			}
			var result struct {
				ExitCode *int   `json:"exit_code"`
				Error    string `json:"error"`
				Stdout   string `json:"stdout"`
			}
			if err := json.Unmarshal([]byte(output), &result); err != nil || result.ExitCode == nil || *result.ExitCode != 0 || result.Error != "" {
				failures++
				batchFailed = true
				lastFailure = output
				sendEvent(ctx, events, conversation.Event{Role: conversation.System, Text: "Python 执行失败，准备修复。"})
			} else {
				pythonSucceeded = true
			}
		}
		if batchFailed {
			pythonPending = true
		} else if pythonSucceeded {
			pythonPending = false
		}
		if failures >= 3 {
			return fmt.Errorf("Python 任务未完成：执行失败 3 次，已停止重试。最后的执行结果：\n%s", lastFailure)
		}
	}
	return fmt.Errorf("达到本轮 8 次模型调用上限，任务尚未完成")
}

// readAnswer 主动读取模型输出，逐段发送给界面；网络错误或取消时释放连接。
func readAnswer(ctx context.Context, client chatClient, messages []openai.ChatCompletionMessage, tools []openai.Tool, events chan<- conversation.Event) (net.StreamResult, error) {
	stream, err := client.OpenStream(ctx, messages, tools)
	if err != nil {
		return net.StreamResult{}, err
	}
	defer stream.Close()
	for {
		if err := ctx.Err(); err != nil {
			return net.StreamResult{}, err
		}
		text, err := stream.Next()
		if errors.Is(err, io.EOF) {
			return stream.Result(), nil
		}
		if err != nil {
			return net.StreamResult{}, err
		}
		if text != "" {
			sendEvent(ctx, events, conversation.Event{Kind: conversation.Delta, Role: conversation.Assistant, Text: text})
		}
	}
}

// sendEvent 发送显示消息；取消时停止等待界面。nil 通道用于不需要显示的单元测试。
func sendEvent(ctx context.Context, events chan<- conversation.Event, event conversation.Event) {
	if events == nil {
		return
	}
	select {
	case events <- event:
	case <-ctx.Done():
	}
}
