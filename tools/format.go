package tools

import (
	"encoding/json"
	"fmt"
	"strings"
)

// InputText 展示模型参数：单个字符串展开为正文，多个参数保留 JSON，避免漏掉 cwd。
func InputText(arguments string) string {
	var fields map[string]json.RawMessage

	if json.Unmarshal([]byte(arguments), &fields) == nil && len(fields) == 1 {
		for _, value := range fields {
			var text string

			if json.Unmarshal(value, &text) == nil {
				return text
			}
		}
	}

	return prettyJSON(arguments)
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
