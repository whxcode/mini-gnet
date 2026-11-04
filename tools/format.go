package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// InputText 原样保留模型参数，包括字段名、空格、换行和转义序列。
func InputText(arguments string) string {
	return arguments
}

// CallText 将调用身份和原始参数分开显示，不把单字段参数简化为一个值。
func CallText(name, callID, arguments string) string {
	return "tool_name: " + name +
		"\ntool_call_id: " + callID +
		"\n\narguments（原始 JSON）:\n" + InputText(arguments)
}

// ExecutionText 展示真实命令数组和 argv，避免拼成 Shell 字符串后丢失参数边界。
func ExecutionText(execution *ExecutionInfo) string {
	if execution == nil {
		return ""
	}

	command := []string{execution.Executable}
	if len(execution.Argv) > 1 {
		command = append(command, execution.Argv[1:]...)
	}

	commandJSON, _ := json.Marshal(command)
	argvJSON, _ := json.MarshalIndent(execution.Argv, "", "  ")
	text := "\n\n执行信息:\ncommand（程序 + 参数）: " + string(commandJSON)
	text += "\nargv（完整原始参数数组）:\n" + string(argvJSON)
	text += "\ncwd: " + execution.Cwd
	text += fmt.Sprintf("\nstarted: %t", execution.Started)

	if execution.ExitCode != nil {
		text += fmt.Sprintf("\nexit_code: %d", *execution.ExitCode)
	}

	text += fmt.Sprintf("\nduration_ms: %d", execution.DurationMS)

	return text
}

// ResultText 在结果中重复调用编号，方便多个工具或重试时对应具体调用。
func ResultText(callID, output string) string {
	return "tool_call_id: " + callID + "\n\n" + OutputText(output)
}

// OutputText 展示统一结果，并兼容旧记录里的纯文本和裸进程结果；不按工具名称分支。
func OutputText(output string) string {
	var result struct {
		OK    *bool           `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error *ToolError      `json:"error"`
	}

	if json.Unmarshal([]byte(output), &result) == nil && result.OK != nil {
		status := "结果：成功"

		if !*result.OK {
			status = "结果：失败"
		}

		if len(result.Data) > 0 && string(result.Data) != "null" {
			status += "\n" + dataText(result.Data)
		}

		if result.Error != nil {
			status += "\n" + result.Error.Code + ": " + result.Error.Message
		}

		return status
	}

	// 旧 Python 历史以 exit_code 为顶层字段，继续保留可读显示。
	var legacy map[string]json.RawMessage

	if json.Unmarshal([]byte(output), &legacy) == nil && legacy["exit_code"] != nil {
		text := dataText([]byte(output))
		var message string

		if json.Unmarshal(legacy["error"], &message) == nil && message != "" {
			text += "\nerror: " + message
		}

		return text
	}

	return output
}

// dataText 识别进程输出结构，其余工具数据以缩进 JSON 显示。
func dataText(data []byte) string {
	var process struct {
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
		ExitCode *int   `json:"exit_code"`
	}

	if json.Unmarshal(data, &process) != nil || process.ExitCode == nil {
		return prettyJSON(string(data))
	}

	text := fmt.Sprintf("退出码：%d", *process.ExitCode)

	if process.Stdout != "" {
		text += "\n" + strings.TrimRight(process.Stdout, "\n")
	}

	if process.Stderr != "" {
		text += "\nstderr:\n" + strings.TrimRight(process.Stderr, "\n")
	}

	return text
}

// prettyJSON 只格式化有效 JSON，不改变原始历史或工具参数。
func prettyJSON(text string) string {
	var value json.RawMessage

	if json.Unmarshal([]byte(text), &value) != nil {
		return text
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return text
	}

	return string(data)
}
