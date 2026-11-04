package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sashabaranov/go-openai"
)

// entry 将已校验的定义和真正的执行入口绑定，声明与可执行工具不会脱节。
type entry struct {
	definition Definition
	tool       Tool
	input      *schema
	output     *schema
}

// Registry 启动时一次构建，之后只读，可供多个会话并发调用。
type Registry struct {
	entries map[string]entry
	order   []string // 保留注册顺序，让模型声明和 /tools 的显示顺序稳定。
}

// NewRegistry 校验并注册所有工具，不扫描文件系统，也不根据名称猜测实现。
func NewRegistry(implementations ...Tool) (*Registry, error) {
	r := &Registry{entries: make(map[string]entry)}

	for _, implementation := range implementations {
		if implementation == nil {
			return nil, fmt.Errorf("工具执行入口不能为空")
		}

		data, err := json.Marshal(implementation.Definition())
		if err != nil {
			return nil, err
		}

		definition, err := ParseDefinition(data)
		if err != nil {
			return nil, err
		}

		if _, exists := r.entries[definition.Name]; exists {
			return nil, fmt.Errorf("工具名称重复：%s", definition.Name)
		}

		input, err := parseSchema(definition.InputSchema)
		if err != nil {
			return nil, err
		}

		output, err := parseSchema(definition.OutputSchema)
		if err != nil {
			return nil, err
		}

		r.entries[definition.Name] = entry{
			definition: definition,
			tool:       implementation,
			input:      input,
			output:     output,
		}
		r.order = append(r.order, definition.Name)
	}

	return r, nil
}

// Definitions 返回定义副本，菜单只能查看，不能改变注册内容。
func (r *Registry) Definitions() []Definition {
	var definitions []Definition

	if r == nil {
		return definitions
	}

	for _, name := range r.order {
		definitions = append(definitions, r.entries[name].definition.Clone())
	}

	return definitions
}

// ModelTools 将统一定义转换为当前模型接口的工具声明，只发送名称、说明和输入约束。
func (r *Registry) ModelTools() []openai.Tool {
	var definitions []openai.Tool

	for _, definition := range r.Definitions() {
		definitions = append(definitions, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        definition.Name,
				Description: definition.Description,
				Parameters:  definition.InputSchema,
			},
		})
	}

	return definitions
}

// failure 将查找、参数、执行和输出错误转成相同的响应外层。
func failure(code string, err error) Result {
	return Result{
		Error: &ToolError{
			Code:    code,
			Message: err.Error(),
		},
	}
}

// Execute 顺序执行：查找、校验参数、调用工具、校验输出、组装结果。
func (r *Registry) Execute(parent context.Context, name string, request Request) Result {
	if r == nil {
		return failure(ToolNotFound, fmt.Errorf("未注册工具：%s", name))
	}

	item, exists := r.entries[name]
	if !exists {
		return failure(ToolNotFound, fmt.Errorf("未注册工具：%s", name))
	}

	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return executionFailure(Result{}, err)
	}

	if err := item.input.validateJSON(request.Arguments); err != nil {
		return failure(InvalidArguments, err)
	}

	result, runErr := item.tool.Execute(ctx, request)

	if ctx.Err() != nil {
		runErr = ctx.Err()
	}

	if result.Data != nil {
		data, err := json.Marshal(result.Data)
		if err != nil {
			result.Data = nil

			return executionFailure(result, &ToolError{
				Code:    InvalidOutput,
				Message: err.Error(),
			})
		}

		if err := item.output.validateJSON(data); err != nil {
			result.Data = nil

			return executionFailure(result, &ToolError{
				Code:    InvalidOutput,
				Message: err.Error(),
			})
		}
	} else if runErr == nil {
		return executionFailure(result, &ToolError{
			Code:    InvalidOutput,
			Message: "工具成功时必须返回符合 output_schema 的 data",
		})
	}

	if runErr != nil {
		return executionFailure(result, runErr)
	}

	result.OK = true
	result.Error = nil

	return result
}

// executionFailure 保留失败时的有效输出，并统一识别取消、超时和工具提供的错误码。
func executionFailure(result Result, err error) Result {
	code := ExecutionFailed
	var toolErr *ToolError

	switch {
	case errors.Is(err, context.Canceled):
		code = Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code = Timeout
	case errors.As(err, &toolErr):
		code = toolErr.Code
	}

	result.OK = false
	result.Error = &ToolError{
		Code:    code,
		Message: err.Error(),
	}

	return result
}
