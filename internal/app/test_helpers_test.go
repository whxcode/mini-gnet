package app

import (
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/tools/builtin"
	"testing"
)

// newTestApp 使用内置工具创建测试应用；外部目录加载由专门的集成测试覆盖。
func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	return newApp(cfg, registry)
}
