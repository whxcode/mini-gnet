package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseSummary 支持常见 YAML 写法，只读取摘要，并拒绝缺失或类型错误的字段。
func TestParseSummary(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		description string
		invalid     bool
	}{
		{
			name:        "quoted",
			content:     "---\nname: code-review\ndescription: '检查代码: 找出问题'\nmetadata:\n  author: demo\n---\n正文不能进入简介。",
			description: "检查代码: 找出问题",
		},
		{
			name:        "folded",
			content:     "---\nname: code-review\ndescription: >\n  第一行\n  第二行\n---\n正文",
			description: "第一行 第二行",
		},
		{
			name:        "literal-crlf-bom",
			content:     "\uFEFF---\r\nname: code-review\r\ndescription: |\r\n  第一行\r\n  第二行\r\n---\r\n正文",
			description: "第一行\n第二行",
		},
		{name: "missing-frontmatter", content: "# 标题", invalid: true},
		{name: "missing-end", content: "---\nname: demo", invalid: true},
		{name: "missing-description", content: "---\nname: demo\n---", invalid: true},
		{name: "empty-name", content: "---\nname: ''\ndescription: demo\n---", invalid: true},
		{name: "non-string", content: "---\nname: demo\ndescription: 123\n---", invalid: true},
		{name: "duplicate-key", content: "---\nname: first\nname: second\ndescription: demo\n---", invalid: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			summary, err := parseDocument([]byte(tc.content))
			if tc.invalid {
				if err == nil {
					t.Fatal("无效元数据没有报错")
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}
			if summary.Name != "code-review" || summary.Description != tc.description {
				t.Fatalf("摘要解析错误：%+v", summary)
			}
		})
	}
}

// TestLoadDir 验证缺失目录、顺序、非技能文件跳过，以及重复名称的具体报错路径。
func TestLoadDir(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "skills")
	summaries, err := LoadDir(directory)
	if err != nil || len(summaries) != 0 {
		t.Fatal("目录不存在时应返回空列表", err)
	}

	for _, name := range []string{"z-last", "a-first", "empty"} {
		path := filepath.Join(directory, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if name == "empty" {
			continue
		}

		content := "---\nname: " + name + "\ndescription: 测试简介\n---\n不注入的正文"
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte("不是技能"), 0o600); err != nil {
		t.Fatal(err)
	}

	summaries, err = LoadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries[0].Name != "a-first" || summaries[1].Name != "z-last" {
		t.Fatalf("扫描结果不正确：%+v", summaries)
	}

	path := filepath.Join(directory, "z-last", "SKILL.md")
	if summaries[1].Path != path {
		t.Fatal("没有保留文件位置")
	}
	if err := os.WriteFile(path, []byte("---\nname: a-first\ndescription: 重复\n---"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(directory); err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "重复") {
		t.Fatal("重复名称应报告文件位置", err)
	}
}
