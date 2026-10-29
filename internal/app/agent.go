package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/net"
	"github.com/whxcode/mini-gnet/views/conversation"
)

// chatClient 只要求模型能打开一个可主动读取的流，测试可以提供假客户端。
type chatClient interface {
	// OpenStream 返回可由 Agent 顺序读取的响应流。
	OpenStream(context.Context, []openai.ChatCompletionMessage, []openai.Tool) (net.ResponseStream, error)
}

// RunTask 由主循环启动：按顺序请求模型、读取流、保存回答、执行工具，再决定是否继续。
// 这个函数会阻塞到任务结束；交互模式用 go 调用，管道模式直接调用。
// job 已由 prepareTask 占用会话并捕获配置，这里不读取 App，也不再启动协程或转发 channel。
func (s *Session) RunTask(job *task) (err error) {
	// 无论下面在哪一步 return，都用返回的 err 更新状态并关闭 done，避免退出时一直等待。
	defer func() { err = s.finishTask(job, err) }()
	ctx := job.ctx
	client := job.client
	tools := job.tools
	projectDir := job.projectDir
	history := s.history
	pythonPending := false
	failures := 0
	reminders := 0
	lastFailure := ""

	for step := 0; step < 8; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		messages := history.ToChatCompletionMessage()
		if pythonPending {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: "上次 run_python 执行失败，用户请求尚未完成。请根据以下错误修正完整 Python 源码，立即再次调用 run_python。不能仅解释错误、展示代码或让用户手动执行。NameError 表示名称未定义，例如使用 os.cpu_count() 前必须实际加入 import os。只修复原任务，不扩大操作范围。\n实际错误：\n" + lastFailure,
			})
		}
		s.updateDisplay(conversation.Event{Kind: conversation.Status, Role: conversation.Assistant, Text: "正在生成 · Ctrl+C 停止"})
		// 1. 发起本轮模型请求，直接拿到可读取的响应流。
		stream, err := client.OpenStream(ctx, messages, tools)
		if err != nil {
			return err
		}

		// 2. 顺序读取片段，直接更新本会话；不经过界面回调或消息中转协程。
		// 一直读取 delta 直到 os.EOF 表示读取完毕
		for {
			if err = ctx.Err(); err != nil {
				break
			}
			var chunk string
			chunk, err = stream.Next()
			if err != nil {
				break
			}
			if chunk != "" {
				s.updateDisplay(conversation.Event{Kind: conversation.Delta, Role: conversation.Assistant, Text: chunk})
			}
		}

		// 每轮读完就关闭连接，避免把所有连接的释放拖到整个任务结束。
		stream.Close()
		if !errors.Is(err, io.EOF) {
			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		// 获取完整的回答和工具调用；未读完的响应不能作为完整消息写入历史。
		answer := stream.Result()

		// 3. 保存完整回答；未读完的响应不能作为完整消息写入历史。
		if err := history.AppendAssistant(answer.Text, answer.ToolCalls); err != nil {
			return err
		}

		// 回显完整回答，避免模型只输出工具调用而界面没有任何显示。
		if strings.TrimSpace(answer.Text) != "" {
			s.updateDisplay(conversation.Event{Role: conversation.Assistant, Text: answer.Text})
		}

		// 4. 没有工具调用就结束；Python 失败时保留有限次修复提醒。
		if len(answer.ToolCalls) == 0 && !pythonPending {
			return nil
		}

		if len(answer.ToolCalls) == 0 {
			if reminders >= 2 {
				return fmt.Errorf("Python 任务未完成：模型仍未调用工具修复错误。最后的执行结果：\n%s", lastFailure)
			}

			reminders++
			s.updateDisplay(conversation.Event{Role: conversation.System, Text: "Python 尚未成功，要求模型继续修复并执行。"})
			continue
		}

		// 5. 按模型给出的顺序执行工具，并把真实结果保存到下一轮上下文。
		batchFailed := false
		pythonSucceeded := false

		for _, call := range answer.ToolCalls {
			s.updateDisplay(conversation.Event{Kind: conversation.Status, Role: conversation.Tool, Name: call.Function.Name, Text: "正在执行 · Ctrl+C 停止"})
			s.updateDisplay(conversation.Event{Role: conversation.Tool, Name: call.Function.Name + " · 调用", Text: toolInputText(call.Function.Name, call.Function.Arguments)})
			// 即使耗尽重试次数，也要为每个工具调用保存结果，保持协议完整。
			output := "Python 重试次数已用尽，本次工具调用未执行。"

			if failures < 3 {
				output = executeTool(ctx, projectDir, call.Function.Name, call.Function.Arguments)
			}

			if err := history.AppendTool(call.ID, output); err != nil {
				return err
			}

			s.updateDisplay(conversation.Event{Role: conversation.Tool, Name: call.Function.Name + " · 结果", Text: toolOutputText(call.Function.Name, output)})

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
				s.updateDisplay(conversation.Event{Role: conversation.System, Text: "Python 执行失败，准备修复。"})
			} else {
				pythonSucceeded = true
			}
		}

		// 6. 更新重试状态，然后回到循环顶部重新请求模型。
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
