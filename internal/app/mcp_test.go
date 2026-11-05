package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/mcp"
	"github.com/whxcode/mini-gnet/tools/builtin"
)

// TestMCPCommandsManageLocalConfig 验证命令持久化、重载且不污染模型历史或工具注册表。
func TestMCPCommandsManageLocalConfig(t *testing.T) {
	a := terminalTestApp(t)
	before := len(a.CurrentSession.History.ToChatCompletionMessage())
	toolCount := len(a.ToolInfos())
	initialConfig := a.ConfigStore().Current()
	var output bytes.Buffer
	input := strings.Join([]string{
		"/mcp",
		"/mcp add docs https://example.invalid/mcp",
		"/mcp add docs https://duplicate.invalid/mcp",
		"/mcp edit docs https://updated.invalid/mcp",
		"/mcp show docs",
		"/mcp search DOCS",
		"/mcp remove missing",
		"/mcp add",
		"/mcp edit docs",
		"/exit",
	}, "\n")

	if err := a.RunPipe(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"没有匹配的服务", "已添加", "已存在", "已更新", "https://updated.invalid/mcp", "未连接", "未配置 MCP 服务 missing", "交互终端"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("缺少 %q：%s", want, output.String())
		}
	}
	if len(a.CurrentSession.History.ToChatCompletionMessage()) != before {
		t.Fatal("MCP 管理污染了模型历史")
	}
	if len(a.ToolInfos()) != toolCount {
		t.Fatal("本地管理阶段不应注册远程工具")
	}
	if a.ConfigStore().Current() != initialConfig {
		t.Fatal("服务管理不应改变主配置")
	}

	manager, err := mcp.Open(a.MCPManager().Path())
	if err != nil {
		t.Fatal(err)
	}
	server, err := manager.Get("docs")
	if err != nil || server.URL != "https://updated.invalid/mcp" {
		t.Fatal("未持久化修改", err)
	}

	executeCommandForTest(t, a, "/mcp remove docs")
	manager, err = mcp.Open(a.MCPManager().Path())
	if err != nil || len(manager.List()) != 0 {
		t.Fatal("删除未持久化", err)
	}
}

// TestAppRejectsBrokenMCP 验证启动时报告配置损坏，不创建会话或覆盖文件。
func TestAppRejectsBrokenMCP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newApp(config.Config{MCPConfigPath: path}, registry); err == nil {
		t.Fatal("损坏的 MCP 配置未阻止启动")
	}
}
