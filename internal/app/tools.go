package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/whxcode/mini-gnet/tools/bash"
	"github.com/whxcode/mini-gnet/tools/run"
)

type bashArguments struct {
	Command string `json:"command"`
}

// executeTool 解析模型参数、运行本地工具，并把成功或失败结果返回给模型。
func executeTool(ctx context.Context, projectDir, name, arguments string) string {
	switch name {
	case "run_bash":
		var args bashArguments
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return "invalid tool arguments: " + err.Error()
		}
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

// 工具协议仍然使用 JSON；界面把参数和 Python 输出展开，避免用户阅读转义后的长字符串。
func toolInputText(name, arguments string) string {
	var args map[string]json.RawMessage
	if json.Unmarshal([]byte(arguments), &args) == nil {
		field := "command"
		if name == "run_python" {
			field = "code"
		}
		var text string
		if json.Unmarshal(args[field], &text) == nil {
			return text
		}
	}
	return arguments
}

// toolOutputText 将工具结果转换成终端可读文本，原始结果仍完整保存。
func toolOutputText(name, output string) string {
	if name != "run_python" {
		return output
	}
	var result struct {
		run.RunResult
		Error string `json:"error"`
	}
	if json.Unmarshal([]byte(output), &result) != nil {
		return output
	}
	text := fmt.Sprintf("退出码：%d", result.ExitCode)
	if result.Stdout != "" {
		text += "\n" + strings.TrimRight(result.Stdout, "\n")
	}
	if result.Stderr != "" {
		text += "\nstderr:\n" + strings.TrimRight(result.Stderr, "\n")
	}
	if result.Error != "" {
		text += "\nerror: " + result.Error
	}
	return text
}
