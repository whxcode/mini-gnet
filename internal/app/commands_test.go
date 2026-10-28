package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/net"
	"github.com/whxcode/mini-gnet/views/commands"
	"github.com/whxcode/mini-gnet/views/message"
)

// 验证菜单结果经过主循环后确实切换上下文，并继续写原文件；失败和 Esc 不换会话。
func TestLocalCommandSessionSwitch(t *testing.T) {
	cfg := config.Config{ChatsDir: t.TempDir(), ProjectDir: t.TempDir()}
	history, err := message.New(cfg.ChatsDir)
	if err != nil {
		t.Fatal(err)
	}
	history.AppendUserMessage("先前的问题")
	id := history.SessionID
	if err := history.Close(); err != nil {
		t.Fatal(err)
	}
	active, err := startSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	app := &App{config: cfg, session: active, client: &net.Client{}}
	defer app.Close()
	original := active
	if err := app.applyLocalCommand(commands.Result{Handled: true}); err != nil || app.session != original {
		t.Fatalf("cancel changed session: %v", err)
	}
	if err := app.applyLocalCommand(commands.Result{SessionID: "missing"}); err == nil || app.session != original {
		t.Fatal("failed restore must preserve current session")
	}
	if err := app.applyLocalCommand(commands.Result{SessionID: id}); err != nil {
		t.Fatal(err)
	}
	if app.session.SessionID != id || app.session.ToChatCompletionMessage()[0].Content != "先前的问题" {
		t.Fatal("history was not restored")
	}
	app.session.AppendUserMessage("继续提问")
	if err := app.session.Err(); err != nil {
		t.Fatal(err)
	}
	if err := app.applyLocalCommand(commands.Result{New: true}); err != nil {
		t.Fatal(err)
	}
	if app.session.SessionID == id || len(app.session.ToChatCompletionMessage()) != 1 {
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
