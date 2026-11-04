package app

import (
	"bytes"
	"context"
	"encoding/json"
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

// TestToolsCommandIncludesExternal 验证菜单显示外部工具路径，但模型声明不包含启动配置。
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
	input := strings.NewReader("/tools\n/tools city_weather\n/tools web_search\n/exit\n")
	if err := application.RunPipe(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"run_bash",
		"run_python",
		"city_weather",
		"city (string，必填)",
		"web_search",
		"query (string，必填)",
		"limit (integer，可选)",
		"output_schema",
		"执行命令\nnode main.mjs",
		"启动参数 · argv\n[\"main.mjs\"]",
		"工具目录\n" + filepath.Join(directory, "web_search"),
		"配置文件\n" + filepath.Join(directory, "web_search", "tool.json"),
		"工作目录 · cwd\n" + filepath.Join(directory, "web_search"),
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("缺少工具信息 %s：%s", want, output.String())
		}
	}

	modelJSON, err := json.Marshal(registry.ModelTools())
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(modelJSON), directory) || strings.Contains(string(modelJSON), "main.mjs") {
		t.Fatal("模型声明不应包含本地启动配置")
	}

	if len(history.ToChatCompletionMessage()) != 1 {
		t.Fatal("/tools 不应增加模型对话")
	}
}
