package command

import (
	"path/filepath"
	"testing"

	"github.com/whxcode/mini-gnet/internal/mcp"
)

// TestMCPInvalidArguments 验证缺失或多余参数不会触发修改，管道不会进入表单。
func TestMCPInvalidArguments(t *testing.T) {
	manager, err := mcp.Open(filepath.Join(t.TempDir(), "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add"}, {"add", "name"}, {"edit"}, {"edit", "name"},
		{"show"}, {"show", "name", "extra"}, {"search"},
		{"remove"}, {"remove", "name", "extra"}, {"unknown"},
	} {
		if _, err := runMCPCommand(manager, args); err == nil {
			t.Fatalf("接受了无效参数 %v", args)
		}
	}
	if len(manager.List()) != 0 {
		t.Fatal("无效命令修改了配置")
	}
	cmd, args, err := Lookup("/mcp add docs https://example.invalid/mcp")
	if err != nil || cmd == nil || cmd.Name != MCPName || len(args) != 3 {
		t.Fatal("命令注册未生效", err)
	}
}
