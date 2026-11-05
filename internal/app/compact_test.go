package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/command"
	"github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/views/message"
)

// compactTestApp 建立已有对话的应用，只连接测试指定的模拟服务。
func compactTestApp(t *testing.T, url string) *App {
	t.Helper()
	cfg := config.Config{BaseURL: url, Model: "test", ProjectDir: t.TempDir(), ChatsDir: t.TempDir()}
	history, err := session.NewHistory(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := history.AppendUserMessage("修复登录，保留公开接口"); err != nil {
		t.Fatal(err)
	}
	if err := history.AppendResponse("已定位 auth.go，尚未验证", nil, openai.FinishReasonStop, true, nil); err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, cfg)
	a.AttachSession(history)
	t.Cleanup(func() { a.Close() })
	return a
}

// TestCompactCreatesIndependentSession 从命令入口验证原文件不变、摘要展示和恢复，以及新问题实际携带摘要。
func TestCompactCreatesIndependentSession(t *testing.T) {
	const summary = "目标：修复登录；约束：保留接口；待办：验证 auth.go。"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if requests.Add(1) == 1 {
			if len(request.Tools) != 0 || !strings.Contains(request.Messages[1].Content, "保留公开接口") {
				t.Error("摘要请求应包含旧历史且不注册工具")
			}
		} else {
			if len(request.Messages) != 3 || !strings.Contains(request.Messages[1].Content, summary) || strings.TrimSpace(request.Messages[2].Content) != "继续验证" {
				t.Errorf("新请求未只携带规则、摘要、新提问，消息数=%d", len(request.Messages))
			}
			data, _ := json.Marshal(request.Messages)
			if strings.Contains(string(data), "summary_source") {
				t.Error("本地来源元数据不应发给模型")
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": summary}, "finish_reason": "stop"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
	}))
	defer server.Close()
	a := compactTestApp(t, server.URL)
	original := a.CurrentSession
	before, err := os.ReadFile(original.History.Path())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := a.RunPipe(context.Background(), strings.NewReader("/compact\n"), &output); err != nil {
		t.Fatal(err)
	}
	if len(a.opened) != 2 || a.CurrentSession == original || requests.Load() != 1 || !strings.Contains(output.String(), summary) {
		t.Fatalf("没有创建并切换摘要会话：%s", output.String())
	}
	next := a.CurrentSession
	records := next.History.Records()
	if len(records) != 2 || records[1].Summary == nil || records[1].Summary.SessionID != original.History.SessionID {
		t.Fatalf("摘要记录缺失或复制了原对话：%+v", records)
	}
	if next.Snapshot().Usage.Last != nil || next.Snapshot().Usage.Reported != 0 {
		t.Fatal("总结旧会话的 token 不应冒充新会话上下文")
	}
	view := ansi.Strip(newTerminal(a, context.Background()).View().Content)
	if !strings.Contains(view, "摘要") {
		t.Fatal("摘要未在新会话显示")
	}

	restored, err := message.Resume(a.config.Current().ChatsDir, next.History.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if got := restored.Records()[1]; got.Summary == nil || !strings.Contains(got.Content, summary) {
		t.Fatal("恢复后摘要或来源丢失")
	}
	entries, err := message.List(a.config.Current().ChatsDir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("摘要会话未进入恢复列表：%v", err)
	}
	for _, entry := range entries {
		if entry.ID == next.History.SessionID && entry.Title != "摘要续聊 · 修复登录，保留公开接口" {
			t.Fatal(entry.Title)
		}
	}
	if err := a.RunPipe(context.Background(), strings.NewReader("继续验证\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(original.History.Path())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("原会话文件被修改", err)
	}
	if err := executeCommandForTest(t, a, "/ps 1"); err != nil || a.CurrentSession != original {
		t.Fatal("无法切回原会话", err)
	}
}

// TestCompactFailuresKeepOriginal 验证 HTTP、空回答、截断和保存失败均不切换或污染原历史。
func TestCompactFailuresKeepOriginal(t *testing.T) {
	for _, kind := range []string{"http", "empty", "length", "save"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if kind == "http" {
					http.Error(w, "model unavailable", http.StatusServiceUnavailable)
					return
				}
				text := "有效摘要"
				finish := "stop"
				if kind == "empty" {
					text = ""
				}
				if kind == "length" {
					finish = "length"
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":%q}]}\n\ndata: [DONE]\n\n", text, finish)
			}))
			defer server.Close()
			a := compactTestApp(t, server.URL)
			original := a.CurrentSession
			before, _ := os.ReadFile(original.History.Path())
			if kind == "save" {
				cfg := a.config.Current()
				cfg.ChatsDir = filepath.Join(t.TempDir(), "not-a-directory")
				if err := os.WriteFile(cfg.ChatsDir, []byte("occupied"), 0600); err != nil {
					t.Fatal(err)
				}
				a.config.Set(cfg)
			}
			cmd, args, err := command.Lookup("/compact")
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.RunPipe(context.Background(), a, args, io.Discard); err == nil {
				t.Fatal("失败未返回错误")
			}
			after, _ := os.ReadFile(original.History.Path())
			if a.CurrentSession != original || len(a.opened) != 1 || !bytes.Equal(before, after) {
				t.Fatal("失败后改变了原会话")
			}
			files, err := filepath.Glob(filepath.Join(filepath.Dir(original.History.Path()), "*.jsonl"))
			if err != nil || len(files) != 1 {
				t.Fatal("失败创建了额外会话文件", err)
			}
		})
	}
}

// TestCompactRejectsBusyAndEmpty 验证空会话及执行中、排队中的会话不会发起摘要请求。
func TestCompactRejectsBusyAndEmpty(t *testing.T) {
	a := terminalTestApp(t)
	cmd, args, _ := command.Lookup("/compact")
	if err := cmd.RunPipe(context.Background(), a, args, io.Discard); err == nil || !strings.Contains(err.Error(), "没有可总结") {
		t.Fatal(err)
	}

	started := make(chan struct{})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		if requests.Add(1) == 1 {
			close(started)
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	cfg := a.config.Current()
	cfg.BaseURL = server.URL
	a.config.Set(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	job, err := a.CurrentSession.Enqueue(ctx, "等待")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("任务未开始")
	}
	for i := 0; i < 2; i++ {
		if err := cmd.RunPipe(ctx, a, args, io.Discard); err == nil || !strings.Contains(err.Error(), "执行或等待") {
			t.Fatal(err)
		}
		if i == 0 {
			if _, err := a.CurrentSession.Enqueue(ctx, "排队"); err != nil {
				t.Fatal(err)
			}
		}
	}
	a.CurrentSession.Stop()
	<-job.Done
	if requests.Load() != 1 {
		t.Fatal("忙碌时不应调用摘要模型")
	}
}

// TestCompactCanceledKeepsOriginal 验证管道取消发生在摘要中途时，不创建或切换会话。
func TestCompactCanceledKeepsOriginal(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"未完成摘要\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	a := compactTestApp(t, server.URL)
	original := a.CurrentSession
	before, err := os.ReadFile(original.History.Path())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd, args, err := command.Lookup("/compact")
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.RunPipe(ctx, a, args, io.Discard) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("摘要未开始")
	}
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("取消未返回：%v", err)
	}
	after, err := os.ReadFile(original.History.Path())
	if err != nil || !bytes.Equal(before, after) || a.CurrentSession != original || len(a.opened) != 1 {
		t.Fatal("取消后改变了原会话", err)
	}
	files, err := filepath.Glob(filepath.Join(a.config.Current().ChatsDir, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatal("取消后留下了摘要会话文件", err)
	}
}
