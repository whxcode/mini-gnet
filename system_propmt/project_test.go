package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildAgentPrompt 验证工作目录规则与摘要合并、正文隔离、缺失目录与下次重新加载。
func TestBuildAgentPrompt(t *testing.T) {
	project := t.TempDir()
	text, err := BuildAgentPrompt(project)
	if err != nil || text != CheckAgentPrompt(project) {
		t.Fatal("没有 .agent 时应保留基础提示词", err)
	}

	directory := filepath.Join(project, ".agent", "skills", "demo")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	rulesPath := filepath.Join(project, ".agent", "AGENTS.md")
	if err := os.WriteFile(rulesPath, []byte("项目规则标记 A"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("根目录规则不自动注入"), 0o600); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(directory, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("---\nname: demo\ndescription: 技能摘要标记\n---\n技能正文不自动注入"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 未传目录时使用进程工作目录；传入目录则使用明确指定的项目。
	t.Chdir(project)
	text, err = BuildAgentPrompt("")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{CheckAgentPrompt(project), "项目规则标记 A", "名称: demo", "技能摘要标记", skillPath} {
		if !strings.Contains(text, want) {
			t.Fatalf("提示词缺少 %q", want)
		}
	}
	for _, unwanted := range []string{"技能正文不自动注入", "根目录规则不自动注入"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("注入了不应包含的内容：%s", unwanted)
		}
	}

	if err := os.WriteFile(rulesPath, []byte("项目规则标记 B"), 0o600); err != nil {
		t.Fatal(err)
	}
	text, err = BuildAgentPrompt(project)
	if err != nil || strings.Contains(text, "项目规则标记 A") || strings.Count(text, "项目规则标记 B") != 1 {
		t.Fatal("再次构建应读取新规则且不重复追加", err)
	}

	other := t.TempDir()
	text, err = BuildAgentPrompt(other)
	if err != nil || strings.Contains(text, "项目规则标记 B") || strings.Contains(text, "技能摘要标记") {
		t.Fatal("另一个工作目录混入了本项目规则", err)
	}
}

// TestBuildAgentPromptInvalidFiles 验证格式和读取错误能带路径返回，不静默忽略。
func TestBuildAgentPromptInvalidFiles(t *testing.T) {
	project := t.TempDir()
	directory := filepath.Join(project, ".agent", "skills", "broken")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "SKILL.md")
	if err := os.WriteFile(path, []byte("缺少元数据"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAgentPrompt(project); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatal("技能格式错误未返回具体路径", err)
	}

	rulesPath := filepath.Join(project, ".agent", "AGENTS.md")
	if err := os.Mkdir(rulesPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildAgentPrompt(project); err == nil || !strings.Contains(err.Error(), rulesPath) {
		t.Fatal("项目规则无法读取时应报错", err)
	}
}
