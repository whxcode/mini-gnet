package builtin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/whxcode/mini-gnet/tools"
)

// TestLoadSkillTool 验证真实注册表的声明、参数和结果校验，以及未知技能和取消处理。
func TestLoadSkillTool(t *testing.T) {
	project := t.TempDir()
	directory := filepath.Join(project, ".agent", "skills", "review")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: review\ndescription: 检查代码\n---\n先查看实际变更。"), 0o600); err != nil {
		t.Fatal(err)
	}

	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := registry.Execute(context.Background(), "load_skill", requestForTest(t, project, map[string]string{"name": "review"}))
	if !result.OK || result.Execution != nil {
		t.Fatalf("技能加载应成功且不启动进程：%+v", result)
	}

	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]string
	if err := json.Unmarshal(encoded, &data); err != nil {
		t.Fatal(err)
	}
	if data["name"] != "review" || data["content"] != "先查看实际变更。" || data["directory"] != directory {
		t.Fatalf("工具输出字段错误：%v", data)
	}

	for _, arguments := range []string{`{}`, `{"name":2}`, `{"name":""}`, `{"name":"review","extra":true}`} {
		result := registry.Execute(context.Background(), "load_skill", tools.Request{
			Arguments: json.RawMessage(arguments),
			Context:   tools.Context{WorkDir: project},
		})
		if result.OK || result.Error == nil || result.Error.Code != tools.InvalidArguments {
			t.Fatalf("参数 %s 未正确拒绝：%+v", arguments, result)
		}
	}

	missing := registry.Execute(context.Background(), "load_skill", requestForTest(t, project, map[string]string{"name": "missing"}))
	if missing.OK || missing.Error == nil {
		t.Fatal("不存在的技能没有返回错误")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := registry.Execute(ctx, "load_skill", requestForTest(t, project, map[string]string{"name": "review"}))
	if canceled.OK || canceled.Error == nil || canceled.Error.Code != tools.Canceled {
		t.Fatal("取消后仍加载技能")
	}
}
