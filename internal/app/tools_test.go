package app

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/tools/builtin"
	"github.com/whxcode/mini-gnet/tools/external"
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

// TestToolsCommandIncludesExternal 验证天气工具与内置工具一起展示，且不向模型暴露启动配置。
func TestToolsCommandIncludesExternal(t *testing.T) {
	directory, err := filepath.Abs("../../my_tools")
	if err != nil {
		t.Fatal(err)
	}

	implementations, err := builtin.Load()
	if err != nil {
		t.Fatal(err)
	}

	externalTools, err := external.LoadDir(directory)
	if err != nil {
		t.Fatal(err)
	}

	implementations = append(implementations, externalTools...)
	registry, err := tools.NewRegistry(implementations...)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		ChatsDir:   t.TempDir(),
		ProjectDir: t.TempDir(),
		ToolsDir:   directory,
	}
	history, err := session.NewHistory(cfg)
	if err != nil {
		t.Fatal(err)
	}

	application := newApp(cfg, registry)
	application.AttachSession(history)
	defer application.Close()

	var output bytes.Buffer
	input := strings.NewReader("/tools\n/tools city_weather\n/exit\n")
	if err := application.RunPipe(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"run_bash", "run_python", "city_weather", "city (string，必填)", "output_schema"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("缺少工具信息 %s：%s", want, output.String())
		}
	}

	if strings.Contains(output.String(), "execution") {
		t.Fatal("公共工具定义不应包含启动配置")
	}

	if len(history.ToChatCompletionMessage()) != 1 {
		t.Fatal("/tools 不应增加模型对话")
	}
}
