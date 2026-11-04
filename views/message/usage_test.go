package message

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

// TestUsageSurvivesResume 验证多轮累计、旧记录缺失、恢复统计及模型上下文隔离。
func TestUsageSurvivesResume(t *testing.T) {
	directory := t.TempDir()
	history, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer history.Close()

	if err := history.AppendAssistant("旧回答，没有统计", nil); err != nil {
		t.Fatal(err)
	}
	for _, usage := range []TokenUsage{
		{InputTokens: 100, OutputTokens: 20, TotalTokens: 120, Model: "first", ContextWindow: 1000},
		{InputTokens: 200, OutputTokens: 30, TotalTokens: 230, Model: "second", ContextWindow: 2000},
	} {
		if err := history.AppendResponse("回答", nil, openai.FinishReasonStop, true, &usage); err != nil {
			t.Fatal(err)
		}
	}

	checkUsage := func(history *TempMessage) {
		t.Helper()
		u := history.Usage()
		if u.InputTokens != 300 || u.OutputTokens != 50 || u.TotalTokens != 350 || u.Reported != 2 || u.Missing != 1 {
			t.Fatalf("累计错误：%+v", u)
		}
		if u.Last == nil || u.Last.InputTokens != 200 || u.Last.ContextWindow != 2000 || u.Last.Model != "second" {
			t.Fatalf("最新请求错误：%+v", u.Last)
		}
		u.Last.InputTokens = 999
		if history.Usage().Last.InputTokens != 200 {
			t.Fatal("读取统计改变了原数据")
		}
	}
	checkUsage(history)

	restored, err := Resume(directory, history.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	checkUsage(restored)

	data, err := json.Marshal(restored.ToChatCompletionMessage())
	if err != nil || strings.Contains(string(data), "input_tokens") || strings.Contains(string(data), "context_window") {
		t.Fatal("统计元数据不应发送给模型", err)
	}

	if err := restored.AppendResponse("不支持 usage 的接口", nil, openai.FinishReasonStop, true, nil); err != nil {
		t.Fatal(err)
	}
	u := restored.Usage()
	if u.Last != nil || u.Missing != 2 || u.TotalTokens != 350 {
		t.Fatal("未上报用量时不能沿用上一轮占比或清零累计")
	}
}
