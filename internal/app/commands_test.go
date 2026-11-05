package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	commands "github.com/whxcode/mini-gnet/internal/command"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/views/message"
)

// executeCommandForTest 使用与主循环相同的方式查找 handler 并传入 App。
func executeCommandForTest(t *testing.T, a *App, text string) error {
	t.Helper()
	cmd, args, err := commands.Lookup(text)
	if err != nil {
		return err
	}

	if cmd == nil {
		t.Fatalf("没有找到命令：%s", text)
	}

	return cmd.Handler(context.Background(), a, args)
}

// TestPSKeepsSessionState 验证快捷编号切换复用原会话，保留草稿与历史，错误编号不改变当前会话。
func TestPSKeepsSessionState(t *testing.T) {
	a := terminalTestApp(t)
	first := a.CurrentSession
	first.Draft.SetValue("未发送的草稿")

	if err := first.History.AppendUserMessage("已有提问"); err != nil {
		t.Fatal(err)
	}

	if err := executeCommandForTest(t, a, "/new"); err != nil {
		t.Fatal(err)
	}

	second := a.CurrentSession

	if err := executeCommandForTest(t, a, "/ps 1"); err != nil {
		t.Fatal(err)
	}

	if a.CurrentSession != first ||
		first.Draft.Value() != "未发送的草稿" ||
		len(first.History.ToChatCompletionMessage()) != 2 {
		t.Fatal("切换应复用原会话及其草稿、历史")
	}

	for _, invalid := range []string{"/ps 0", "/ps 3", "/ps unknown"} {
		if err := executeCommandForTest(t, a, invalid); err == nil || a.CurrentSession != first {
			t.Fatalf("错误编号不应改变当前会话：%s", invalid)
		}
	}

	if err := executeCommandForTest(t, a, "/ps 2"); err != nil || a.CurrentSession != second || len(a.opened) != 2 {
		t.Fatal("快捷切换不应新建会话", err)
	}
}

// TestExitHandlerChangesApp 验证退出命令直接更新应用状态，不再返回需要二次处理的 Result。
func TestExitHandlerChangesApp(t *testing.T) {
	a := &App{}

	if err := executeCommandForTest(t, a, "/exit"); err != nil {
		t.Fatal(err)
	}

	if !a.quit {
		t.Fatal("退出 handler 没有更新 App")
	}
}

// TestLocalCommandSessionSwitch 验证找到的 handler 直接切换 App 会话、继续写原文件；失败保留原状态。
func TestLocalCommandSessionSwitch(t *testing.T) {
	cfg := config.Config{
		ChatsDir:   t.TempDir(),
		ProjectDir: t.TempDir(),
	}
	history, err := message.New(cfg.ChatsDir)
	if err != nil {
		t.Fatal(err)
	}

	if err := history.AppendUserMessage("先前的问题"); err != nil {
		t.Fatal(err)
	}

	id := history.SessionID

	if err := history.Close(); err != nil {
		t.Fatal(err)
	}

	active, err := sessions.NewHistory(cfg)
	if err != nil {
		t.Fatal(err)
	}

	app := newTestApp(t, cfg)
	app.AttachSession(active)
	defer app.Close()

	original := active

	if err := executeCommandForTest(t, app, "/resume missing"); err == nil || app.CurrentSession.History != original {
		t.Fatal("failed restore must preserve current session")
	}

	if err := executeCommandForTest(t, app, "/resume "+id); err != nil {
		t.Fatal(err)
	}

	if app.CurrentSession.History.SessionID != id ||
		app.CurrentSession.History.ToChatCompletionMessage()[0].Content != "先前的问题" {
		t.Fatal("history was not restored")
	}

	if err := app.CurrentSession.History.AppendUserMessage("继续提问"); err != nil {
		t.Fatal(err)
	}

	if err := executeCommandForTest(t, app, "/new"); err != nil {
		t.Fatal(err)
	}

	if app.CurrentSession.History.SessionID == id ||
		len(app.CurrentSession.History.ToChatCompletionMessage()) != 1 {
		t.Fatal("new session retained old context")
	}

	restored, err := message.Resume(cfg.ChatsDir, id)
	if err != nil {
		t.Fatal(err)
	}

	defer restored.Close()

	if len(restored.ToChatCompletionMessage()) != 2 {
		t.Fatal("restored conversation was not appended")
	}

	if _, err := os.Stat(filepath.Join(cfg.ChatsDir, original.SessionID+".jsonl")); err != nil {
		t.Fatal("switching discarded previous history", err)
	}
}
