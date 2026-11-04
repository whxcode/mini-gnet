package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCallTextPreservesArguments 验证字段名、空白和转义序列不会在显示时被简化。
func TestCallTextPreservesArguments(t *testing.T) {
	arguments := " { \"city\" : \"成都\", \"code\": \"print(1)\\nprint(2)\" } "
	text := CallText("city_weather", "call-weather-1", arguments)

	if !strings.HasSuffix(text, arguments) ||
		!strings.Contains(text, "tool_name: city_weather") ||
		!strings.Contains(text, "tool_call_id: call-weather-1") {
		t.Fatalf("调用信息被改写：%s", text)
	}

	if InputText(`{"city":"成都"}`) != `{"city":"成都"}` {
		t.Fatal("单字段 JSON 不应简化成字段值")
	}
}

// TestExecutionDetailsStayLocal 验证完整命令的参数边界可辨认，执行元数据不会进入模型结果。
func TestExecutionDetailsStayLocal(t *testing.T) {
	code := 0
	execution := &ExecutionInfo{
		Executable: "/runtime with spaces/python3",
		Argv:       []string{"python3", "main.py", "a b", `a"b`, "$HOME"},
		Cwd:        "/my tools/weather",
		Started:    true,
		ExitCode:   &code,
		DurationMS: 12,
	}
	text := ExecutionText(execution)

	for _, want := range []string{
		`["/runtime with spaces/python3","main.py","a b","a\"b","$HOME"]`,
		"cwd: /my tools/weather",
		"started: true",
		"exit_code: 0",
		"duration_ms: 12",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("执行详情缺少 %q：%s", want, text)
		}
	}

	data, err := json.Marshal(Result{
		OK:        true,
		Data:      ProcessOutput{Execution: execution},
		Execution: execution,
	})
	if err != nil || strings.Contains(string(data), "python3") || strings.Contains(string(data), "duration_ms") {
		t.Fatalf("本地执行信息进入了工具协议：%s %v", data, err)
	}
}
