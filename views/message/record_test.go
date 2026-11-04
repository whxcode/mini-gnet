package message

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/internal/query"
)

// TestResponseMetadataSurvivesResume 验证新旧记录混用、恢复时保留最终标记，发给模型时移除本地字段。
func TestResponseMetadataSurvivesResume(t *testing.T) {
	dir := t.TempDir()
	history, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := history.AppendAssistant("旧记录", nil); err != nil {
		t.Fatal(err)
	}

	if err := history.AppendResponse("中间说明", nil, openai.FinishReasonToolCalls, false, nil); err != nil {
		t.Fatal(err)
	}

	if err := history.AppendResponse("最终正文", nil, openai.FinishReasonStop, true, nil); err != nil {
		t.Fatal(err)
	}

	if err := history.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := Resume(dir, history.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	defer restored.Close()

	records := restored.Records()

	if len(records) != 3 ||
		records[0].Response != nil ||
		records[1].Response == nil ||
		records[1].Response.Final ||
		records[2].Response == nil ||
		!records[2].Response.Final ||
		records[2].Response.Finish != openai.FinishReasonStop {
		t.Fatalf("结束标记未正确恢复：%+v", records)
	}

	data, err := json.Marshal(restored.ToChatCompletionMessage())
	if err != nil ||
		strings.Contains(string(data), "agent_response") ||
		!strings.Contains(string(data), "最终正文") {
		t.Fatal("本地显示标记进入了模型上下文，或正文丢失", err)
	}
}

// TestUserInputMetadata 验证原始片段恢复、调用方副本隔离以及模型只收到正常 user 内容。
func TestUserInputMetadata(t *testing.T) {
	directory := t.TempDir()
	history, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	parts := query.Input{{Kind: query.Skill, Value: "review"}}
	if err := history.AppendInputMessage("已组装的正文", parts); err != nil {
		t.Fatal(err)
	}
	parts[0].Value = "外部修改"
	snapshot := history.Records()
	snapshot[0].Input[0].Value = "快照修改"
	if err := history.Close(); err != nil {
		t.Fatal(err)
	}

	restored, err := Resume(directory, history.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if restored.Records()[0].Input[0].Value != "review" {
		t.Fatal("原始引用被外部修改或未保存")
	}
	data, err := json.Marshal(restored.ToChatCompletionMessage())
	if err != nil || strings.Contains(string(data), "user_input") || !strings.Contains(string(data), "已组装的正文") {
		t.Fatal("本地片段进入模型协议或正文丢失", err)
	}
}
