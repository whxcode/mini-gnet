package message

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sashabaranov/go-openai"
)

func TestResumePreservesToolsAndAppends(t *testing.T) {
	dir := t.TempDir()
	m, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	m.AppendSystemMessage("system")
	m.AppendUserMessage("第一条问题")
	m.AppendAssistant("", []openai.ToolCall{{ID: "call-1", Type: "function", Function: openai.FunctionCall{Name: "run_python", Arguments: `{"code":"print(1)"}`}}})
	m.AppendTool("call-1", strings.Repeat("长工具结果", 20000))
	id := m.SessionID
	want := m.ToChatCompletionMessage()
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	resumed, err := Resume(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(want)
	b, _ := json.Marshal(resumed.ToChatCompletionMessage())
	if string(a) != string(b) {
		t.Fatal("恢复丢失历史正文或 tool_calls 关联")
	}
	resumed.AppendUserMessage("继续上一条")
	if err := resumed.Err(); err != nil {
		t.Fatal(err)
	}
	resumed.Close()
	again, err := Resume(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.ToChatCompletionMessage(); len(got) != 5 || got[4].Content != "继续上一条" {
		t.Fatal("继续聊天没有追加到原会话")
	}
}

func TestHistoryListAndCorruption(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"old", "new"} {
		path := filepath.Join(dir, id+".jsonl")
		if err := os.WriteFile(path, []byte(`{"role":"user","content":"标题"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(dir, "old.jsonl"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.jsonl"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := List(dir)
	if err != nil || len(list) != 3 {
		t.Fatalf("列表读取: %+v %v", list, err)
	}
	if list[len(list)-1].ID != "old" {
		t.Fatal("未按更新时间排序")
	}
	for _, s := range list {
		if s.ID == "broken" && s.Problem == "" {
			t.Fatal("损坏记录没有标记")
		}
	}
	if _, err := Resume(dir, "broken"); err == nil {
		t.Fatal("损坏记录不能恢复并追加")
	}
	if _, err := Resume(dir, "../escape"); err == nil {
		t.Fatal("不应允许目录穿越")
	}
	// 旧文件没有末尾换行时，新消息仍应写成独立的一行。
	m, err := Resume(dir, "new")
	if err != nil {
		t.Fatal(err)
	}
	m.AppendAssistant("回答", nil)
	m.Close()
	got, _, err := readHistory(filepath.Join(dir, "new.jsonl"))
	if err != nil || len(got) != 2 {
		t.Fatalf("无末尾换行的文件追加失败: %v", err)
	}
}
