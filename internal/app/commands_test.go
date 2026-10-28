package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/net"
	"github.com/whxcode/mini-gnet/views/message"
)

// executeCommandForTest 使用与主循环相同的方式查找 handler 并传入 App。
func executeCommandForTest(t *testing.T, a *App, text string) error {
	t.Helper()
	cmd, args, err := lookupCommand(text)
	if err != nil {
		return err
	}
	if cmd == nil {
		t.Fatalf("没有找到命令：%s", text)
	}
	return cmd.Handler(a, args)
}

// TestLookupCommand 验证主循环能拿到命令与参数，普通提问不被识别为命令。
func TestLookupCommand(t *testing.T) {
	for _, tc := range []struct {
		text    string
		name    commandName
		args    string
		wantErr bool
	}{
		{"/help", commandHelpName, "", false},
		{"/config", commandConfigName, "", false},
		{"/new", commandNewName, "", false},
		{"/exit", commandExitName, "", false},
		{"/resume", commandResumeName, "", false},
		{"  /resume\told-id  ", commandResumeName, "old-id", false},
		{"/unknown", "", "", true},
		{"/help unexpected", "", "", true},
		{"/config unexpected", "", "", true},
		{"/new unexpected", "", "", true},
		{"/exit unexpected", "", "", true},
		{"/resume first second", "", "", true},
		{"/Users/mac/project 下有什么", "", "", false},
		{"普通问题", "", "", false},
		{"/help\n这是一段代码", "", "", false},
		{"", "", "", false},
	} {
		cmd, args, err := lookupCommand(tc.text)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q: err=%v, wantErr=%v", tc.text, err, tc.wantErr)
		}
		if tc.name == "" {
			if cmd != nil {
				t.Errorf("%q 不应找到命令", tc.text)
			}
			continue
		}
		if cmd == nil || cmd.Name != tc.name || cmd.Handler == nil || strings.Join(args, " ") != tc.args {
			t.Errorf("%q: cmd=%+v, args=%v", tc.text, cmd, args)
		}
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

	if err := executeCommandForTest(t, app, "/resume missing"); err == nil || app.session != original {
		t.Fatal("failed restore must preserve current session")
	}
	if err := executeCommandForTest(t, app, "/resume "+id); err != nil {
		t.Fatal(err)
	}
	if app.session.SessionID != id || app.session.ToChatCompletionMessage()[0].Content != "先前的问题" {
		t.Fatal("history was not restored")
	}
	app.session.AppendUserMessage("继续提问")
	if err := app.session.Err(); err != nil {
		t.Fatal(err)
	}
	if err := executeCommandForTest(t, app, "/new"); err != nil {
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
