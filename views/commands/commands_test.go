package commands

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/views/message"
)

func TestMenusEscapeWithoutApplying(t *testing.T) {
	cfg := config.Config{BaseURL: "http://localhost:11434/v1", Model: "original", APIKey: "test-hidden-secret", ChatsDir: t.TempDir()}
	m := newConfig(cfg)
	if strings.Contains(ansi.Strip(m.View().Content), cfg.APIKey) {
		t.Fatal("配置界面泄露了 API Key")
	}
	m.fields[1].SetValue("changed")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := next.(configModel); !got.done || got.saved != nil || cfg.Model != "original" {
		t.Fatal("Esc 不应应用配置")
	}
	r := newResume([]message.SessionInfo{{ID: "first"}, {ID: "second"}}, "first")
	next, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	r = next.(resumeModel)
	next, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := next.(resumeModel); !got.done || got.selected != "" {
		t.Fatal("Esc 不应切换会话")
	}
	next, _ = newHelp().Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !next.(helpModel).done {
		t.Fatal("帮助菜单未返回")
	}
}

func TestResumeSelectionAndEmpty(t *testing.T) {
	r := newResume([]message.SessionInfo{{ID: "first"}, {ID: "second"}, {ID: "bad", Problem: "损坏"}}, "first")
	next, _ := r.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	r = next.(resumeModel)
	next, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := next.(resumeModel); got.selected != "second" || !got.done {
		t.Fatal("上下键 + Enter 未选中会话")
	}
	r.cursor = 2
	next, _ = r.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := next.(resumeModel); got.done || got.problem == "" {
		t.Fatal("损坏会话不应被选中")
	}
	next, _ = newResume(nil, "").Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if next.(resumeModel).done {
		t.Fatal("空列表不应选中")
	}
}

func TestConfigSaveAndValidation(t *testing.T) {
	cfg := config.Config{BaseURL: "http://localhost:11434/v1", Model: "original", ChatsDir: t.TempDir()}
	m := newConfig(cfg)
	m.focus = len(m.fields)
	m.fields[1].SetValue("")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(configModel)
	if m.done || m.err == "" {
		t.Fatal("空模型名称应留在表单中报错")
	}
	m.fields[1].SetValue("new-model")
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := next.(configModel); !got.done || got.saved == nil || got.saved.Model != "new-model" {
		t.Fatal("未返回已确认的配置")
	}
}

func TestLocalCommandRouting(t *testing.T) {
	for _, tc := range []struct {
		text    string
		handled bool
	}{
		{"/new", true}, {"/exit", true}, {"/resume old-id", true}, {"/unknown", true},
		{"/Users/mac/project 下有什么", false}, {"普通问题", false}, {"/help\n这是一段代码", false},
	} {
		result, _ := Handle(tc.text, config.Config{}, "")
		if result.Handled != tc.handled {
			t.Fatalf("%q 路由错误", tc.text)
		}
	}
}
