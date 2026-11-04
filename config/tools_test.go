package config

import (
	"path/filepath"
	"testing"
)

// TestToolsDirectory 验证默认值、项目相对路径和绝对路径，并兼容旧配置文件。
func TestToolsDirectory(t *testing.T) {
	project := t.TempDir()
	absolute := filepath.Join(t.TempDir(), "external")
	base := Config{
		BaseURL:    "http://localhost:11434/v1",
		Model:      "test",
		ChatsDir:   t.TempDir(),
		ProjectDir: project,
	}
	cases := []struct {
		path string
		want string
	}{
		{path: "", want: filepath.Join(project, "my_tools")},
		{path: "custom", want: filepath.Join(project, "custom")},
		{path: "../shared", want: filepath.Join(project, "..", "shared")},
		{path: absolute, want: absolute},
	}

	for _, test := range cases {
		cfg := base
		cfg.ToolsDir = test.path
		normalized, err := Normalize(cfg)
		if err != nil || normalized.ToolsDir != test.want {
			t.Fatalf("目录 %q：得到 %q，期望 %q，错误 %v", test.path, normalized.ToolsDir, test.want, err)
		}
	}

	path := filepath.Join(t.TempDir(), "config.json")
	base.ToolsDir = absolute
	if err := saveFile(path, base); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadFile(path, Config{})
	if err != nil || loaded.ToolsDir != absolute {
		t.Fatalf("toolsDir 没有保存或重载：%v", err)
	}
}

// TestToolsDirectoryUsesStartupDirectory 验证未指定项目目录时以启动目录寻找 my_tools。
func TestToolsDirectoryUsesStartupDirectory(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)

	cfg, err := Normalize(Config{
		BaseURL:  "http://localhost:11434/v1",
		Model:    "test",
		ChatsDir: directory,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := filepath.EvalSymlinks(filepath.Dir(cfg.ToolsDir))
	want, _ := filepath.EvalSymlinks(directory)
	if got != want || filepath.Base(cfg.ToolsDir) != "my_tools" {
		t.Fatalf("默认工具目录不在启动目录：%s", cfg.ToolsDir)
	}
}
