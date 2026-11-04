package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadByMetadataName 验证按摘要名称加载正文，目录名不同也能定位，不能通过参数指定任意文件。
func TestLoadByMetadataName(t *testing.T) {
	project := t.TempDir()
	directory := filepath.Join(project, ".agent", "skills", "review-folder")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(directory, "SKILL.md")
	content := "---\nname: code-review\ndescription: 检查代码\n---\n# 审查步骤\n\n先读取 references/rules.md。\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	document, err := Load(project, "code-review")
	if err != nil {
		t.Fatal(err)
	}
	if document.Path != path || document.Directory != directory || document.Content != "# 审查步骤\n\n先读取 references/rules.md。" {
		t.Fatalf("正文或相对资源基准错误：%+v", document)
	}

	query := document.WithQuery("检查这次修改")
	if strings.Count(query, document.Content) != 1 || !strings.Contains(query, "检查这次修改") || !strings.Contains(query, directory) {
		t.Fatal("手动提问没有完整保留问题、正文或资源位置")
	}

	for _, name := range []string{"", "review-folder", "missing", "../review-folder/SKILL.md", path} {
		if _, err := Load(project, name); err == nil {
			t.Fatalf("未知名称 %q 不应加载成功", name)
		}
	}

	if err := os.WriteFile(path, []byte("---\nname: code-review\ndescription: 检查代码\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(project, "code-review"); err == nil || !strings.Contains(err.Error(), "没有正文") {
		t.Fatal("空正文应明确报错", err)
	}
}
