package message

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
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

	if err := history.AppendResponse("中间说明", nil, openai.FinishReasonToolCalls, false); err != nil {
		t.Fatal(err)
	}

	if err := history.AppendResponse("最终正文", nil, openai.FinishReasonStop, true); err != nil {
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
