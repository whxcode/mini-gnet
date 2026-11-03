package app

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestToolsCommandUsesRegistry 验证 /tools 列出真实定义、支持详情和未知名称提示，且不污染模型历史。
func TestToolsCommandUsesRegistry(t *testing.T) {
	a := terminalTestApp(t)
	before := len(a.CurrentSession.History.ToChatCompletionMessage())
	var output bytes.Buffer
	input := "/tools\n/tools run_bash\n/tools missing\n/exit\n"

	if err := a.RunPipe(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"run_bash",
		"run_python",
		"command (string，必填)",
		"cwd (string，可选)",
		"output_schema",
		"未注册工具 missing",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("工具说明缺少 %s：%s", want, output.String())
		}
	}

	if len(a.CurrentSession.History.ToChatCompletionMessage()) != before {
		t.Fatal("/tools 被写入模型对话")
	}
}
