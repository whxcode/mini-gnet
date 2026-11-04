package query

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestParseReferences 验证显式语法、中文边界、代码示例以及路径不会混为引用。
func TestParseReferences(t *testing.T) {
	input, err := Parse("请使用 /skills{review} 检查")
	want := Input{
		{Kind: Text, Value: "请使用 "},
		{Kind: Skill, Value: "review"},
		{Kind: Text, Value: " 检查"},
	}
	if err != nil || !reflect.DeepEqual(input, want) {
		t.Fatalf("解析结果错误：%+v, %v", input, err)
	}
	for _, text := range []string{
		"`/skills{review}`", "```go\n/skills{review}\n```", "~~~\n/skills{review}\n~~~",
		"https://example.org/skills{review}", "/tmp/skills{review}",
	} {
		input, err := Parse(text)
		if err != nil || !reflect.DeepEqual(input, Input{{Kind: Text, Value: text}}) {
			t.Fatalf("普通代码或路径被解释为引用：%q, %+v, %v", text, input, err)
		}
	}
	for _, text := range []string{"/skills{", "/skills{}", "/skills{a\nb}", "/skills{{a}"} {
		if _, err := Parse(text); err == nil {
			t.Fatalf("无效引用未报错：%q", text)
		}
	}
}

// TestResolveReadsCurrentBody 验证在执行时读取正文、同任务去重、缺失技能和取消不会伪装成功。
func TestResolveReadsCurrentBody(t *testing.T) {
	project := t.TempDir()
	directory := filepath.Join(project, ".agent", "skills", "review")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "SKILL.md")
	input, err := Parse("使用 /skills{review} 和 /skills{review} 检查")
	if err != nil {
		t.Fatal(err)
	}

	for _, version := range []string{"正文 A", "正文 B"} {
		if err := os.WriteFile(path, []byte("---\nname: review\ndescription: 审查\n---\n"+version), 0o600); err != nil {
			t.Fatal(err)
		}
		content, err := input.Resolve(context.Background(), project)
		if err != nil || strings.Count(content, version) != 1 || !strings.Contains(content, directory) {
			t.Fatalf("没有读取当前正文或重复注入：%q, %v", content, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := input.Resolve(ctx, project); err == nil {
		t.Fatal("取消后仍然完成组装")
	}
	missing := Input{{Kind: Skill, Value: "missing"}}
	if _, err := missing.Resolve(context.Background(), project); err == nil {
		t.Fatal("缺失技能未报错")
	}
	if err := (Input{{Kind: "unknown", Value: "x"}}).Validate(); err == nil {
		t.Fatal("未知类型未报错")
	}
}
