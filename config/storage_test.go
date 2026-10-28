package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLoadAndInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings", "config.json")
	defaults := Config{BaseURL: "http://localhost:11434/v1", Model: "old", ChatsDir: dir, ProjectDir: dir}
	got, err := loadFile(path, defaults)
	if err != nil || got.Model != "old" {
		t.Fatalf("首次启动默认配置失败: %+v %v", got, err)
	}
	want := defaults
	want.Model = "new"
	want.APIKey = "test-secret-not-a-real-key"
	if err := saveFile(path, want); err != nil {
		t.Fatal(err)
	}
	got, err = loadFile(path, defaults)
	if err != nil || got != want {
		t.Fatalf("保存后重载不一致: %+v %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("配置文件权限应为 0600: %v", err)
		}
	}
	want.BaseURL = "not a URL"
	if err := saveFile(path, want); err == nil {
		t.Fatal("无效配置不应保存")
	}
	got, err = loadFile(path, defaults)
	if err != nil || got.Model != "new" || got.BaseURL != defaults.BaseURL {
		t.Fatal("失败保存破坏了原文件")
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadFile(path, defaults); err == nil {
		t.Fatal("损坏配置不应被静默忽略")
	}
}
