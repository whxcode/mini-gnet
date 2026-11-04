package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	"github.com/whxcode/mini-gnet/views/message"
)

// skillTestApp 建立隔离项目和真实会话文件，模型请求只发送到本地测试服务。
func skillTestApp(t *testing.T, url string) *App {
	t.Helper()
	cfg := config.Config{
		ProjectDir: t.TempDir(),
		ChatsDir:   t.TempDir(),
		BaseURL:    url,
		Model:      "test",
	}
	directory := filepath.Join(cfg.ProjectDir, ".agent", "skills", "review")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: code-review\ndescription: 审查代码变更\n---\n技能正文验证标记：检查实际代码后报告问题。"), 0o600); err != nil {
		t.Fatal(err)
	}

	history, err := sessions.NewHistory(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := newTestApp(t, cfg)
	a.AttachSession(history)
	t.Cleanup(func() { a.Close() })

	return a
}

// TestManualSkillHistory 验证命令选择、会话隔离、失败保留，以及下一条问题和历史正文的正常持久化。
func TestManualSkillHistory(t *testing.T) {
	requests := make(chan openai.ChatCompletionRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	a := skillTestApp(t, server.URL)
	first := a.CurrentSession
	before := first.History.ToChatCompletionMessage()

	for _, command := range []string{"/skills code-review", "/new", "/ps 1"} {
		if err := executeCommandForTest(t, a, command); err != nil {
			t.Fatal(err)
		}
	}
	if first.Snapshot().SelectedSkill != "code-review" || a.opened[1].Snapshot().SelectedSkill != "" {
		t.Fatal("待选技能没有隔离在所属会话")
	}
	if err := executeCommandForTest(t, a, "/skills missing"); err == nil {
		t.Fatal("未知技能应报错")
	}
	if _, err := first.Enqueue(context.Background(), "  "); err == nil {
		t.Fatal("空问题应报错")
	}
	if first.Snapshot().SelectedSkill != "code-review" || !reflect.DeepEqual(before, first.History.ToChatCompletionMessage()) {
		t.Fatal("选择或失败提交不应修改历史或丢失待选状态")
	}

	// 待选提示增加一行后，窄窗口中的输入光标仍应落在编辑文本上。
	model := newTerminal(a, context.Background())
	model = updateTerminal(model, tea.WindowSizeMsg{Width: 40, Height: 24})
	model.session.Draft.SetValue("编辑中")
	view := model.View()
	lines := strings.Split(ansi.Strip(view.Content), "\n")
	if !strings.Contains(ansi.Strip(view.Content), "下一条使用技能") || len(lines) > 24 || view.Cursor == nil || view.Cursor.Position.Y >= len(lines) || !strings.Contains(lines[view.Cursor.Position.Y], "编辑中") {
		t.Fatal("待选提示影响输入区布局")
	}

	if err := a.RunPipe(context.Background(), strings.NewReader("检查修改\n普通问题\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	if first.Snapshot().SelectedSkill != "" || first.Snapshot().Status != "已完成" {
		t.Fatal("提交后应清除待选且完成任务")
	}

	initial := <-requests
	firstQuestion := initial.Messages[len(initial.Messages)-1]
	if firstQuestion.Role != "user" || !strings.Contains(firstQuestion.Content, "检查修改") || strings.Count(firstQuestion.Content, "技能正文验证标记") != 1 {
		t.Fatal("手动技能没有作为普通 user 消息发送")
	}
	next := <-requests
	if strings.TrimSpace(next.Messages[len(next.Messages)-1].Content) != "普通问题" {
		t.Fatal("第二条提问仍然附加了待选技能")
	}
	if !strings.Contains(next.Messages[1].Content, "技能正文验证标记") {
		t.Fatal("后续请求不应剥离历史技能正文")
	}

	for _, command := range []string{"/skills code-review", "/skills --clear"} {
		if err := executeCommandForTest(t, a, command); err != nil {
			t.Fatal(err)
		}
	}
	if first.Snapshot().SelectedSkill != "" {
		t.Fatal("清除命令没有生效")
	}
	assertSkillHistoryResumes(t, a)

	if err := executeCommandForTest(t, a, "/new"); err != nil {
		t.Fatal(err)
	}
	if len(a.CurrentSession.History.ToChatCompletionMessage()) != 1 || a.CurrentSession.Snapshot().SelectedSkill != "" {
		t.Fatal("新会话仍携带旧提问或待选技能")
	}
}

// TestAutomaticSkillHistory 从模型工具调用验证正文回传、继续回答和后续任务保留历史。
func TestAutomaticSkillHistory(t *testing.T) {
	requests := make(chan openai.ChatCompletionRequest, 3)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"skill-call","type":"function","function":{"name":"load_skill","arguments":"{\"name\":\"code-review\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"审查完成"},"finish_reason":"stop"}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	a := skillTestApp(t, server.URL)
	if err := a.RunPipe(context.Background(), strings.NewReader("检查代码\n另一个问题\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || a.CurrentSession.Snapshot().Status != "已完成" {
		t.Fatal("没有完成加载技能后的模型循环")
	}

	first := <-requests
	if strings.Contains(first.Messages[0].Content, "技能正文验证标记") {
		t.Fatal("没有加载时就把完整正文放进了系统提示词")
	}
	for i := 0; i < 2; i++ {
		request := <-requests
		found := false
		for index, msg := range request.Messages {
			if msg.Role != "tool" || msg.ToolCallID != "skill-call" {
				continue
			}
			var result struct {
				OK   bool `json:"ok"`
				Data struct {
					Content string `json:"content"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(msg.Content), &result); err != nil {
				t.Fatal(err)
			}
			if !result.OK || !strings.Contains(result.Data.Content, "技能正文验证标记") || index == 0 || len(request.Messages[index-1].ToolCalls) != 1 {
				t.Fatal("技能结果或对应工具调用丢失")
			}
			found = true
		}
		if !found {
			t.Fatal("本轮或后续问题没有保留正常 tool 消息")
		}
	}
	assertSkillHistoryResumes(t, a)
}

// assertSkillHistoryResumes 验证技能正文按原消息格式落盘，恢复无需专用技能字段或重写历史。
func assertSkillHistoryResumes(t *testing.T, a *App) {
	t.Helper()
	history := a.CurrentSession.History
	restored, err := message.Resume(a.config.Current().ChatsDir, history.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if !reflect.DeepEqual(history.ToChatCompletionMessage(), restored.ToChatCompletionMessage()) {
		t.Fatal("恢复会话后消息发生变化")
	}
}
