package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whxcode/mini-gnet/config"
)

// TestNewHistoryIncludesProjectRules 确认新会话 JSONL 的系统消息也包含项目规则，便于手工核对。
func TestNewHistoryIncludesProjectRules(t *testing.T) {
	project := t.TempDir()
	directory := filepath.Join(project, ".agent")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "AGENTS.md"), []byte("测试项目规则"), 0o600); err != nil {
		t.Fatal(err)
	}

	history, err := NewHistory(config.Config{ProjectDir: project, ChatsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer history.Close()

	records := history.Records()
	if len(records) != 1 || records[0].Role != "system" || !strings.Contains(records[0].Content, "测试项目规则") {
		t.Fatal("创建会话时未保存项目规则")
	}
}
