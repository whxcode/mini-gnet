package config

import (
	"path/filepath"
	"testing"
)

// TestContextWindowConfiguration 验证默认未知、保存恢复和负数校验，不推断模型理论上限。
func TestContextWindowConfiguration(t *testing.T) {
	t.Setenv("MINI_GO_API_KEY", "")
	directory := t.TempDir()
	cfg := Config{
		BaseURL:    "http://localhost:11434/v1",
		Model:      "qwen3:8b",
		ChatsDir:   directory,
		ProjectDir: directory,
	}

	normalized, err := Normalize(cfg)
	if err != nil || normalized.ContextWindow != 0 {
		t.Fatalf("旧配置应保留未知上限：%v", err)
	}

	cfg.ContextWindow = 8192
	path := filepath.Join(directory, "config.json")
	if err := saveFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	restored, err := loadFile(path, Config{})
	if err != nil || restored.ContextWindow != 8192 {
		t.Fatalf("没有保存上下文上限：%v", err)
	}

	cfg.ContextWindow = -1
	if _, err := Normalize(cfg); err == nil {
		t.Fatal("不能接受负数上限")
	}
}
