package run

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/whxcode/mini-gnet/tools"
)

// Python 定义嵌入二进制，工具加载与启动目录无关。
//
//go:embed schema.json
var definitionJSON []byte

// Tool 将临时 Python 执行器接入统一工具接口。
type Tool struct {
	definition tools.Definition
}

// New 加载 Python 的内置定义，不启动解释器。
func New() (*Tool, error) {
	definition, err := tools.ParseDefinition(definitionJSON)
	if err != nil {
		return nil, err
	}

	return &Tool{definition: definition}, nil
}

// Definition 返回独立的定义副本。
func (t *Tool) Definition() tools.Definition {
	return t.definition.Clone()
}

// Execute 将模型参数交给现有临时脚本执行器，保留失败时的标准输出和错误输出。
func (t *Tool) Execute(ctx context.Context, request tools.Request) (tools.Result, error) {
	var args struct {
		Code string `json:"code"`
	}

	if err := json.Unmarshal(request.Arguments, &args); err != nil {
		return tools.Result{}, &tools.ToolError{
			Code:    tools.InvalidArguments,
			Message: err.Error(),
		}
	}

	if strings.TrimSpace(args.Code) == "" {
		return tools.Result{}, &tools.ToolError{
			Code:    tools.InvalidArguments,
			Message: "code 不能为空",
		}
	}

	output, err := PythonCode(ctx, args.Code)

	return tools.Result{Data: output}, err
}
