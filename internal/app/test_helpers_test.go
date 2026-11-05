package app

import (
	"path/filepath"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/tools/builtin"
)

// newTestApp 使用内置工具创建测试应用；外部目录加载由专门的集成测试覆盖。
func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.MCPConfigPath == "" {
		cfg.MCPConfigPath = filepath.Join(t.TempDir(), "mcp.json")
	}

	app, err := newApp(cfg, registry)
	if err != nil {
		t.Fatal(err)
	}

	return app
}
