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
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/query"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	tasks "github.com/whxcode/mini-gnet/internal/task"
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

// TestManualSkillHistory 验证行内引用、草稿隔离、任务副本和正常历史持久化。
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
	source := "请使用 /skills{code-review} 检查修改"
	first.Draft.SetValue(source)

	for _, command := range []string{"/skills", "/new", "/ps 1"} {
		if err := executeCommandForTest(t, a, command); err != nil {
			t.Fatal(err)
		}
	}
	if first.Draft.Value() != source || a.opened[1].Draft.Value() != "" {
		t.Fatal("技能引用没有保存在所属会话草稿")
	}
	if !reflect.DeepEqual(before, first.History.ToChatCompletionMessage()) {
		t.Fatal("编辑或查看技能目录不应写入历史")
	}

	model := newTerminal(a, context.Background())
	model = updateTerminal(model, tea.WindowSizeMsg{Width: 40, Height: 24})
	view := model.View()
	plain := ansi.Strip(view.Content)
	lines := strings.Split(plain, "\n")
	if !strings.Contains(plain, "◆ code-review") || strings.Contains(ansi.Strip(first.Draft.View()), "/skills{") || len(lines) > 24 || view.Cursor == nil || view.Cursor.Position.Y >= len(lines) {
		t.Fatalf("行内技能或光标布局错误：\n%s", plain)
	}

	job, err := first.EnqueueInput(context.Background(), first.Draft.Segments())
	if err != nil {
		t.Fatal(err)
	}
	first.Draft.SetValue("提交后修改的草稿")
	if err := waitTask(t, job); err != nil {
		t.Fatal(err)
	}
	if err := a.RunPipe(context.Background(), strings.NewReader("普通问题\n"), io.Discard); err != nil {
		t.Fatal(err)
	}
	initial := <-requests
	firstQuestion := initial.Messages[len(initial.Messages)-1]
	if firstQuestion.Role != "user" || !strings.Contains(firstQuestion.Content, "检查修改") || strings.Count(firstQuestion.Content, "技能正文验证标记") != 1 || strings.Contains(firstQuestion.Content, "提交后修改") {
		t.Fatal("任务没有使用独立输入副本或未附加正文")
	}
	next := <-requests
	if strings.TrimSpace(next.Messages[len(next.Messages)-1].Content) != "普通问题" || !strings.Contains(next.Messages[1].Content, "技能正文验证标记") {
		t.Fatal("后续提问不应重复附加技能，也不应删除历史正文")
	}
	assertSkillHistoryResumes(t, a)

	if err := executeCommandForTest(t, a, "/new"); err != nil {
		t.Fatal(err)
	}
	if len(a.CurrentSession.History.ToChatCompletionMessage()) != 1 || a.CurrentSession.Draft.Value() != "" {
		t.Fatal("新会话仍携带旧问题或标签")
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

// TestQueuedSkillReadsAtExecution 验证排队不读取正文，执行开始读取一次，此后的工具轮次复用同一条 user 消息。
func TestQueuedSkillReadsAtExecution(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	requests := make(chan openai.ChatCompletionRequest, 3)
	var calls atomic.Int32
	var skillPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		switch calls.Add(1) {
		case 1:
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		case 2:
			// 这一轮已加载 B，下一轮不得在同任务中悄悄读取 C。
			if err := os.WriteFile(skillPath, []byte("---\nname: code-review\ndescription: 审查\n---\n执行中修改为 C"), 0o600); err != nil {
				t.Error(err)
			}
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"pwd-call","type":"function","function":{"name":"run_bash","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\ndata: [DONE]\n\n")
			return
		}
		fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"完成"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	a := skillTestApp(t, server.URL)
	skillPath = filepath.Join(a.config.Current().ProjectDir, ".agent", "skills", "review", "SKILL.md")
	first, err := a.CurrentSession.Enqueue(context.Background(), "占位任务")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("占位任务未启动")
	}
	parts, err := query.Parse("使用 /skills{code-review} 检查")
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.CurrentSession.EnqueueInput(context.Background(), parts)
	if err != nil {
		t.Fatal(err)
	}
	parts[1].Value = "提交后被编辑器修改的名称"
	if err := os.WriteFile(skillPath, []byte("---\nname: code-review\ndescription: 审查\n---\n排队期间更新为 B"), 0o600); err != nil {
		t.Fatal(err)
	}
	close(release)
	for _, job := range []*tasks.Task{first, second} {
		if err := waitTask(t, job); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("工具循环没有继续执行")
	}
	<-requests
	for range 2 {
		request := <-requests
		found := false
		for _, msg := range request.Messages {
			if msg.Role == "user" && strings.Contains(msg.Content, "【用户指定技能") {
				found = strings.Contains(msg.Content, "排队期间更新为 B") && !strings.Contains(msg.Content, "修改为 C")
			}
		}
		if !found {
			t.Fatal("技能加载时机错误，或任务输入不是独立副本")
		}
	}
}
