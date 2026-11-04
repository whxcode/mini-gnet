// Package skill 将技能正文加载接入统一工具注册表，不执行技能附带的脚本。
package skill

import (
	"context"
	_ "embed"
	"encoding/json"

	skills "github.com/whxcode/mini-gnet/internal/skill"
	"github.com/whxcode/mini-gnet/tools"
)

//go:embed schema.json
var definitionJSON []byte

// Tool 是按名称读取技能正文的内置工具，不持有会话状态。
type Tool struct {
	definition tools.Definition
}

// New 加载嵌入的工具定义，启动阶段即可发现声明错误。
func New() (*Tool, error) {
	definition, err := tools.ParseDefinition(definitionJSON)
	if err != nil {
		return nil, err
	}

	return &Tool{definition: definition}, nil
}

// Definition 返回独立副本，供模型和 /tools 查看。
func (t *Tool) Definition() tools.Definition {
	return t.definition.Clone()
}

// Execute 与手动选择共用技能加载函数，结果由 Agent 作为普通 tool 消息保存。
func (t *Tool) Execute(ctx context.Context, request tools.Request) (tools.Result, error) {
	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	var args struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(request.Arguments, &args); err != nil {
		return tools.Result{}, &tools.ToolError{
			Code:    tools.InvalidArguments,
			Message: err.Error(),
		}
	}

	document, err := skills.Load(request.Context.WorkDir, args.Name)
	if err != nil {
		return tools.Result{}, err
	}

	if err := ctx.Err(); err != nil {
		return tools.Result{}, err
	}

	return tools.Result{Data: document}, nil
}
