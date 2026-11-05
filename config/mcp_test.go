package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMCPConfigPath 验证旧配置兼容默认路径，以及自定义路径的展开与保存。
func TestMCPConfigPath(t *testing.T) {
	t.Setenv("MINI_GO_API_KEY", "")
	cfg := Config{
		BaseURL:  "http://localhost:11434/v1",
		Model:    "test",
		ChatsDir: t.TempDir(),
	}
	got, err := Normalize(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mainPath, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if got.MCPConfigPath != filepath.Join(filepath.Dir(mainPath), "mcp.json") {
		t.Fatal("默认路径没有跟随平台配置目录")
	}

	cfg.MCPConfigPath = "~/custom/mcp.json"
	got, err = Normalize(cfg)
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got.MCPConfigPath != filepath.Join(home, "custom", "mcp.json") {
		t.Fatal("未展开用户目录")
	}

	cfg.MCPConfigPath = filepath.Join(t.TempDir(), "separate", "mcp.json")
	path := filepath.Join(t.TempDir(), "config.json")
	if err := saveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err = loadFile(path, Config{})
	if err != nil || got.MCPConfigPath != cfg.MCPConfigPath {
		t.Fatal("保存重载丢失 MCP 路径", err)
	}
	if _, err := os.Stat(cfg.MCPConfigPath); !os.IsNotExist(err) {
		t.Fatal("主配置不应创建 MCP 文件")
	}
}
