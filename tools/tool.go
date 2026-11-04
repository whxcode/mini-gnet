// Package tools 定义所有工具共用的调用协议，不区分工具来源或实现语言。
package tools

import (
	"context"
	"encoding/json"
)

// Definition 描述一个工具，输入和输出 schema 分别约束 arguments 和结果 data。
type Definition struct {
	SchemaVersion int             `json:"schema_version"` // 当前定义格式版本为 1。
	Name          string          `json:"name"`           // 注册表内唯一名称。
	Description   string          `json:"description"`    // 模型和 /tools 共用的说明。
	InputSchema   json.RawMessage `json:"input_schema"`   // 模型提供的参数结构。
	OutputSchema  json.RawMessage `json:"output_schema"`  // 工具返回的业务数据结构。
}

// Clone 复制 schema 字节，避免调用方修改注册表持有的定义。
func (d Definition) Clone() Definition {
	d.InputSchema = append(json.RawMessage(nil), d.InputSchema...)
	d.OutputSchema = append(json.RawMessage(nil), d.OutputSchema...)

	return d
}

// Request 将模型参数与程序上下文分开；默认目录不是访问边界。
type Request struct {
	Arguments json.RawMessage `json:"arguments"` // 模型生成的 JSON 参数。
	Context   Context         `json:"context"`   // 程序注入，模型无需填写。
}

// Context 提供本次任务的默认位置；工具可通过自身参数选择其他位置。
type Context struct {
	WorkDir string `json:"work_dir"`
}

// Tool 将描述与实现绑定，Agent 只通过这个接口执行。
type Tool interface {
	Definition() Definition
	Execute(context.Context, Request) (Result, error)
}

// Result 是统一返回格式；工具填入 Data 并返回 Go error，注册表统一填写 OK 和 Error。
type Result struct {
	OK    bool       `json:"ok"`
	Data  any        `json:"data"`
	Error *ToolError `json:"error"`

	Execution *ExecutionInfo `json:"-"` // 本地执行信息单独保存，不改变工具输出 schema。
}

// ToolError 用稳定代码供程序判断，用文字说明供模型和用户阅读。
type ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error 使工具可以直接返回带标准错误码的 Go error。
func (e *ToolError) Error() string {
	return e.Message
}

const (
	ToolNotFound     = "TOOL_NOT_FOUND"
	InvalidArguments = "INVALID_ARGUMENTS"
	ExecutionFailed  = "EXECUTION_FAILED"
	Timeout          = "TIMEOUT"
	Canceled         = "CANCELED"
	InvalidOutput    = "INVALID_OUTPUT"
	InternalError    = "INTERNAL_ERROR"
)

// ProcessOutput 是现有两个进程工具的 data；其他工具可以定义自己的业务数据。
type ProcessOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`

	Execution *ExecutionInfo `json:"-"`
}
