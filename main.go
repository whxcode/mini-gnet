package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/user"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	imp "github.com/whxcode/mini-gnet/interface"
	"github.com/whxcode/mini-gnet/logger"
	"github.com/whxcode/mini-gnet/net"
	prompt "github.com/whxcode/mini-gnet/system_propmt"
	"github.com/whxcode/mini-gnet/tools/bash"
	toolsList "github.com/whxcode/mini-gnet/tools/list"
	"github.com/whxcode/mini-gnet/tools/run"
	"github.com/whxcode/mini-gnet/views/message"
)

type bashArguments struct {
	Command string `json:"command"`
}

func executeTool(ctx context.Context, projectDir, name, arguments string) string {
	switch name {
	case "run_bash":
		var args bashArguments
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "invalid tool arguments: " + err.Error()
		}
		logger.Info("执行: ", args.Command)
		output, err := bash.Run(ctx, projectDir, args.Command)
		if err != nil {
			output += "\n" + err.Error()
		}
		return output
	case "run_python":
		var args struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "invalid tool arguments: " + err.Error()
		}
		logger.Info("执行临时 Python:\n", args.Code)
		result, err := run.PythonCode(ctx, args.Code)
		response := struct {
			run.RunResult
			Error string `json:"error,omitempty"`
		}{RunResult: result}
		if err != nil {
			response.Error = err.Error()
		}
		output, _ := json.Marshal(response)
		return string(output)
	default:
		return "unknown tool: " + name
	}
}

type chatClient interface {
	Stream(context.Context, []openai.ChatCompletionMessage, []openai.Tool, func(string) error) (net.StreamResult, error)
}

// runAgentTurn keeps failed Python execution pending until a retry succeeds.
// Reminders are request-local so they cannot affect the next user task.
func runAgentTurn(ctx context.Context, client chatClient, msg imp.Message, tools []openai.Tool, projectDir string) error {
	pythonPending := false
	failures, reminders := 0, 0
	lastFailure := ""
	for step := 0; step < 8; step++ {
		messages := msg.ToChatCompletionMessage()
		if pythonPending {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: "上次 run_python 执行失败，用户请求尚未完成。请根据以下错误修正完整 Python 源码，立即再次调用 run_python。不能仅解释错误、展示代码或让用户手动执行。NameError 表示名称未定义，例如使用 os.cpu_count() 前必须实际加入 import os。只修复原任务，不扩大操作范围。\n实际错误：\n" + lastFailure,
			})
		}
		answer, err := client.Stream(ctx, messages, tools, func(string) error { return nil })
		if err != nil {
			return err
		}
		msg.AppendAssistant(answer.Text, answer.ToolCalls)
		if len(answer.ToolCalls) == 0 {
			if pythonPending {
				if reminders >= 2 {
					return fmt.Errorf("Python 任务未完成：模型仍未调用工具修复错误。最后的执行结果：\n%s", lastFailure)
				}
				reminders++
				logger.Info("Python 尚未成功，要求模型继续修复并执行。")
				continue
			}
			logger.Assistant(answer.Text)
			return nil
		}
		batchFailed := false
		pythonSucceeded := false
		for _, call := range answer.ToolCalls {
			// Always reply to every call, even when the retry budget is exhausted.
			output := "Python 重试次数已用尽，本次工具调用未执行。"
			if failures < 3 {
				output = executeTool(ctx, projectDir, call.Function.Name, call.Function.Arguments)
			}
			msg.AppendTool(call.ID, output)
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
				logger.Info("Python 执行失败，准备修复。\n", output)
			} else {
				pythonSucceeded = true
				logger.Info("Python 执行成功，实际输出：\n", result.Stdout)
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

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic:", r)
			os.Exit(-1)
		}
	}()

	config := config.LoadConfig()
	if config.ProjectDir == "" {
		config.ProjectDir, _ = os.Getwd()
	}
	tools, err := toolsList.GetToolsList()
	if err != nil {
		logger.Error(err)
		return
	}
	user, _ := user.Current()
	userName := user.Username

	if err := os.MkdirAll(config.ChatsDir, os.ModePerm); err != nil {
		panic(err)
	}

	var msg *message.TempMessage = message.MakeTempMessage()

	msg.AppendSystemMessage(prompt.CheckAgentPrompt())

	client := &net.Client{
		BaseURL: config.BaseURL,
		ApiKey:  config.APIKey,
		Model:   config.Model,
		HTTP:    &http.Client{},
	}

	// loop cli
	reader := bufio.NewReader(os.Stdin)
	for {
		logger.User("", userName+"说:----------------------")
		query, err := reader.ReadString('\n')
		if err != nil {
			logger.Error(err)
			return
		}
		msg.AppendUserMessage(query)

		if err := runAgentTurn(context.Background(), client, msg, tools, config.ProjectDir); err != nil {
			logger.Error(err)
		}
	}
}
