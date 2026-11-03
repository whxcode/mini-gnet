package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// testTool 模拟实现用于确认校验发生在执行前，错误输出不会作为成功结果返回。
type testTool struct {
	definition Definition
	calls      int
	data       any
	err        error
}

// Definition 提供测试工具的公开描述。
func (t *testTool) Definition() Definition {
	return t.definition
}

// Execute 记录是否真正执行，用于验证参数错误不会触发工具。
func (t *testTool) Execute(context.Context, Request) (Result, error) {
	t.calls++

	return Result{Data: t.data}, t.err
}

// newTestTool 创建仅接受 text 字符串的工具，输出也按同一结构校验。
func newTestTool() *testTool {
	schema := json.RawMessage(`{"type":"object","properties":{"text":{"type":"string","minLength":1}},"required":["text"],"additionalProperties":false}`)

	return &testTool{
		definition: Definition{
			SchemaVersion: 1,
			Name:          "echo",
			Description:   "echo text",
			InputSchema:   schema,
			OutputSchema:  schema,
		},
		data: map[string]string{"text": "ok"},
	}
}

// TestRegistryRejectsInvalidCalls 验证查找和输入校验先于执行，定义副本也不能篡改校验规则。
func TestRegistryRejectsInvalidCalls(t *testing.T) {
	tool := newTestTool()
	registry, err := NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}

	definitions := registry.Definitions()
	definitions[0].InputSchema[0] = '!'

	for _, arguments := range []string{
		`{}`,
		`{"text":1}`,
		`{"text":""}`,
		`{"text":"ok","unexpected":true}`,
		`{"text":"ok"} {}`,
	} {
		result := registry.Execute(context.Background(), "echo", Request{Arguments: json.RawMessage(arguments)})

		if result.OK || result.Error.Code != InvalidArguments || tool.calls != 0 {
			t.Fatalf("错误参数触发执行：%s %+v", arguments, result)
		}
	}

	unknown := registry.Execute(context.Background(), "missing", Request{Arguments: json.RawMessage(`{}`)})

	if unknown.Error.Code != ToolNotFound || tool.calls != 0 {
		t.Fatal("未知工具处理错误")
	}

	valid := registry.Execute(context.Background(), "echo", Request{Arguments: json.RawMessage(`{"text":"ok"}`)})

	if !valid.OK || valid.Error != nil || tool.calls != 1 {
		t.Fatalf("有效调用失败：%+v", valid)
	}

	tool.data = map[string]int{"text": 123}
	bad := registry.Execute(context.Background(), "echo", Request{Arguments: json.RawMessage(`{"text":"ok"}`)})

	if bad.OK || bad.Data != nil || bad.Error.Code != InvalidOutput {
		t.Fatalf("无效输出未阻止：%+v", bad)
	}
}

// TestRegistryDefinitionAndErrors 验证重复名称和未支持的 schema 会在启动时报错。
func TestRegistryDefinitionAndErrors(t *testing.T) {
	tool := newTestTool()

	if _, err := NewRegistry(tool, tool); err == nil {
		t.Fatal("允许了重复工具名")
	}

	tool.definition.InputSchema = json.RawMessage(`{"type":"object","pattern":"ignored?"}`)

	if _, err := NewRegistry(tool); err == nil {
		t.Fatal("未知 schema 关键字被静默忽略")
	}

	tool = newTestTool()
	registry, err := NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}

	tool.err = errors.New("execution failed")
	result := registry.Execute(context.Background(), "echo", Request{Arguments: json.RawMessage(`{"text":"ok"}`)})

	if result.OK || result.Error.Code != ExecutionFailed || result.Data == nil {
		t.Fatal("执行失败没有保留有效数据")
	}

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	result = registry.Execute(ctx, "echo", Request{Arguments: json.RawMessage(`{"text":"ok"}`)})

	if result.Error.Code != Timeout || tool.calls != 1 {
		t.Fatal("超时请求仍执行了工具")
	}
}
