// Package agent 实现模型与工具的顺序执行循环，不管理队列或终端。
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/net"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/views/message"
)

// Agent 是应用共用的模型执行服务，不持有当前会话或任务队列。
type Agent struct {
	config *config.Store   // 所有会话共享的运行配置。
	tools  *tools.Registry // 共用的定义和执行入口，Agent 不关心工具的具体实现。
}

// New 创建共用的 Agent；它不启动协程，也不创建网络连接。
func New(cfg *config.Store, registry *tools.Registry) *Agent {
	return &Agent{
		config: cfg,
		tools:  registry,
	}
}

// Run 由会话消费循环同步调用：保存提问、请求模型、读取流、执行工具，直到本项结束。
// 本函数只返回执行错误，队列、状态收尾和完成通知由 Session 负责。
func (a *Agent) Run(job *task.Task, history *message.TempMessage, display *Output) error {
	ctx := job.Context

	if err := ctx.Err(); err != nil {
		return err
	}

	// 轮到本任务才写入提问，当前任务不会提前读到后面排队的问题。
	if err := history.AppendUserMessage(job.Query); err != nil {
		return err
	}

	display.Update(Event{
		Role: User,
		Text: job.Query,
	})

	// 任务对象不存配置或工具。配置在本次执行开始时读取一次，避免同一轮工具循环中途换模型。
	cfg := a.config.Current()
	modelTools := a.tools.ModelTools() // 工具列表在应用启动时加载，之后只读，所有会话共用。
	projectDir := cfg.ProjectDir
	toolPending := false
	failures := 0
	lastFailure := ""

	for step := 0; step < 8; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		messages := history.ToChatCompletionMessage()

		// 恢复旧会话也使用本次任务的工作目录，避免历史提示词继续指向旧目录。
		system := openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: prompt.CheckAgentPrompt(projectDir),
		}

		if len(messages) > 0 && messages[0].Role == openai.ChatMessageRoleSystem {
			messages[0] = system
		} else {
			messages = append([]openai.ChatCompletionMessage{system}, messages...)
		}

		if toolPending {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: "上次工具执行失败。请根据实际错误修正参数，在需要时再次调用合适的工具；只处理原任务，不扩大操作范围。\n实际错误：\n" + lastFailure,
			})
		}

		display.Update(Event{
			Kind: Status,
			Role: Assistant,
			Text: "正在生成 · Ctrl+C 停止",
		})

		// 1. 发起本轮模型请求，直接拿到可读取的响应流。
		// 客户端只在请求处临时创建；Session 和 task 不保存、不传递它。
		client := net.Client{
			BaseURL: cfg.BaseURL,
			ApiKey:  cfg.APIKey,
			Model:   cfg.Model,
		}
		stream, err := client.OpenStream(ctx, messages, modelTools)
		if err != nil {
			return err
		}

		// 2. 顺序读取片段，直接更新本会话；不经过界面回调或消息中转协程。
		// 一直读取 delta，直到 io.EOF 表示读取完毕。
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
				display.Update(Event{
					Kind: Delta,
					Role: Assistant,
					Text: chunk,
				})
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

		// 3. 只接受正常结束或完整工具调用；EOF 本身不是任务完成标记。
		// length / content_filter / 缺少结束原因时保留屏幕上的片段，由 Session 标记未完成。
		switch answer.Finish {
		case openai.FinishReasonStop:
		case openai.FinishReasonToolCalls:
			if len(answer.ToolCalls) == 0 {
				return fmt.Errorf("模型返回 tool_calls，但没有提供工具调用，任务未完成")
			}
		default:
			return fmt.Errorf("模型响应未正常结束（finish_reason=%q），任务未完成", answer.Finish)
		}

		final := answer.Finish == openai.FinishReasonStop && len(answer.ToolCalls) == 0

		if err := history.AppendResponse(answer.Text, answer.ToolCalls, answer.Finish, final); err != nil {
			return err
		}

		// 4. 最终回答只按协议判断，不根据正文判断用户需求是否真的解决。
		if final {
			display.Update(Event{
				Role:  Assistant,
				Text:  answer.Text,
				Stage: Final,
			})

			return nil
		}

		display.Update(Event{
			Role:  Assistant,
			Text:  answer.Text,
			Stage: Process,
		})

		// 5. 按模型给出的顺序执行工具，并把真实结果保存到下一轮上下文。
		batchFailed := false

		for _, call := range answer.ToolCalls {
			display.Update(Event{
				Kind: Status,
				Role: Tool,
				Name: call.Function.Name,
				Text: "正在执行 · Ctrl+C 停止",
			})
			display.Update(Event{
				Role: Tool,
				Name: call.Function.Name + " · 调用",
				Text: tools.InputText(call.Function.Arguments),
			})
			result := tools.Result{
				Error: &tools.ToolError{
					Code:    tools.ExecutionFailed,
					Message: "工具重试次数已用尽，本次调用未执行。",
				},
			}

			if failures < 3 {
				result = a.tools.Execute(ctx, call.Function.Name, tools.Request{
					Arguments: json.RawMessage(call.Function.Arguments),
					Context:   tools.Context{WorkDir: projectDir},
				})
			}

			encoded, err := json.Marshal(result)
			if err != nil {
				return fmt.Errorf("编码工具结果：%w", err)
			}

			output := string(encoded)

			if err := history.AppendTool(call.ID, output); err != nil {
				return err
			}

			display.Update(Event{
				Role: Tool,
				Name: call.Function.Name + " · 结果",
				Text: tools.OutputText(output),
			})

			if !result.OK {
				failures++
				batchFailed = true
				lastFailure = output
				display.Update(Event{
					Role:  System,
					Text:  "工具执行失败，结果将交给模型处理。",
					Stage: Process,
				})
			}
		}

		// 6. 更新重试状态，然后回到循环顶部重新请求模型。
		toolPending = batchFailed

		if failures >= 3 {
			return fmt.Errorf("工具任务未完成：执行失败 3 次，已停止重试。最后的执行结果：\n%s", lastFailure)
		}
	}

	return fmt.Errorf("达到本轮 8 次模型调用上限，任务尚未完成")
}
