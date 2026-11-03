// Package builtin 只负责组装程序自带工具，Agent 不依赖这个包。
package builtin

import (
	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/tools/bash"
	"github.com/whxcode/mini-gnet/tools/run"
)

// NewRegistry 是内置工具唯一注册位置；每项都同时提供 schema 和执行入口。
func NewRegistry() (*tools.Registry, error) {
	bashTool, err := bash.New()
	if err != nil {
		return nil, err
	}

	pythonTool, err := run.New()
	if err != nil {
		return nil, err
	}

	return tools.NewRegistry(bashTool, pythonTool)
}
