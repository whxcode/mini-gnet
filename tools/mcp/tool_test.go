package mcp

import (
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/whxcode/mini-gnet/tools"
)

// TestRemoteAliasesAndRegistry 验证同名远程工具隔离、完整 schema 保留、重复注册拒绝和共享表不变。
func TestRemoteAliasesAndRegistry(t *testing.T) {
	definition := &sdk.Tool{
		Name: strings.Repeat("search.long-name", 8),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "enum": []string{"Go", "C"}},
			},
		},
	}
	first, err := Bind("docs", nil, []*sdk.Tool{definition})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Bind("other", nil, []*sdk.Tool{definition})
	if err != nil {
		t.Fatal(err)
	}
	base, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	taskTools, err := base.WithRemote(append(first, second...)...)
	if err != nil {
		t.Fatal(err)
	}
	definitions := taskTools.Definitions()
	if len(base.Definitions()) != 0 || len(definitions) != 2 || definitions[0].Name == definitions[1].Name {
		t.Fatal("远程别名冲突或修改了基础注册表")
	}
	for _, got := range definitions {
		if len(got.Name) > 64 || !strings.Contains(string(got.InputSchema), "enum") {
			t.Fatal("名称超过模型约束或 schema 被修改")
		}
	}
	if _, err := taskTools.WithRemote(first...); err == nil {
		t.Fatal("重复名称应拒绝，不能覆盖执行入口")
	}
}
