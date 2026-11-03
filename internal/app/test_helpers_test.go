package app

import (
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/tools/builtin"
	"testing"
)

// newTestApp 与正式入口使用同一注册过程，不依赖测试进程的工作目录。
func newTestApp(t *testing.T, cfg config.Config) *App {
	t.Helper()
	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	return newApp(cfg, registry)
}
