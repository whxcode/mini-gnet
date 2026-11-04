package bash

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/whxcode/mini-gnet/tools"
)

// 定义随程序编译，运行时不读取启动目录中的 schema.json。
//
//go:embed schema.json
var definitionJSON []byte

// Tool 将 Bash 的定义和执行实现绑定。
type Tool struct {
	definition tools.Definition
}

// New 加载内置 schema；错误在启动阶段返回，不推迟到模型调用时。
func New() (*Tool, error) {
	definition, err := tools.ParseDefinition(definitionJSON)
	if err != nil {
		return nil, err
	}

	return &Tool{definition: definition}, nil
}

// Definition 返回工具定义副本，供注册表组装模型声明。
func (t *Tool) Definition() tools.Definition {
	return t.definition.Clone()
}

// Execute 解析本工具参数并调用 Bash；不修改主进程目录。
func (t *Tool) Execute(ctx context.Context, request tools.Request) (tools.Result, error) {
	var args struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}

	if err := json.Unmarshal(request.Arguments, &args); err != nil {
		return tools.Result{}, &tools.ToolError{
			Code:    tools.InvalidArguments,
			Message: err.Error(),
		}
	}

	if err := validate(args.Command); err != nil {
		return tools.Result{}, &tools.ToolError{
			Code:    tools.InvalidArguments,
			Message: err.Error(),
		}
	}

	cwd, err := resolveDirectory(request.Context.WorkDir, args.Cwd)
	if err != nil {
		return tools.Result{}, &tools.ToolError{
			Code:    tools.InvalidArguments,
			Message: err.Error(),
		}
	}

	output, err := runCommand(ctx, cwd, args.Command)

	return tools.Result{
		Data:      output,
		Execution: output.Execution,
	}, err
}

// resolveDirectory 只决定相对路径的起点，允许上层目录及其他绝对路径。
func resolveDirectory(defaultDir, cwd string) (string, error) {
	if cwd == "" {
		cwd = defaultDir
	} else if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(defaultDir, cwd)
	}

	path, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	if !info.IsDir() {
		return "", &tools.ToolError{
			Code:    tools.InvalidArguments,
			Message: "cwd 必须是目录",
		}
	}

	return path, nil
}
