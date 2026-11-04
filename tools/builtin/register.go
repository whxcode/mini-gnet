// Package builtin 只负责组装程序自带工具，Agent 不依赖这个包。
package builtin

import (
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/tools/bash"
	"github.com/whxcode/mini-gnet/tools/run"
)

// Load 创建程序自带的工具，调用方可以追加外部工具后统一注册。
func Load() ([]tools.Tool, error) {
	bashTool, err := bash.New()
	if err != nil {
		return nil, err
	}

	pythonTool, err := run.New()
	if err != nil {
		return nil, err
	}

	return []tools.Tool{bashTool, pythonTool}, nil
}

// NewRegistry 创建仅含内置工具的注册表，供独立调用和测试使用。
func NewRegistry() (*tools.Registry, error) {
	implementations, err := Load()
	if err != nil {
		return nil, err
	}

	return tools.NewRegistry(implementations...)
}
