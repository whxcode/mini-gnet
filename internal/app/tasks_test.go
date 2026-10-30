package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/sashabaranov/go-openai"
	"github.com/whxcode/mini-gnet/config"
)

// TestQueuedTaskStopsOnHistoryError 验证入队后保存提问失败会结束本任务，不请求模型或显示提交成功。
func TestQueuedTaskStopsOnHistoryError(t *testing.T) {
	a := terminalTestApp(t)
	s := a.CurrentSession
	before := s.snapshot()
	if err := s.history.Close(); err != nil {
		t.Fatal(err)
	}
	job, err := a.enqueueTask(context.Background(), "不能保存的提问")
	if err != nil {
		t.Fatal(err)
	}
	if err := waitTask(t, job); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("应返回保存错误：%v", err)
	}
	after := s.snapshot()
	if after.Running || len(s.history.ToChatCompletionMessage()) != 1 || after.Status != "失败" {
		t.Fatal("保存失败后不应启动模型或写入提问")
	}
	if len(after.Messages) != len(before.Messages)+1 {
		t.Fatal("应只显示一条失败信息")
	}
}

// TestBackgroundCompletesWithoutSwitchBack 验证不切回且不刷新界面，后台仍能收完长流并写入磁盘。
func TestBackgroundCompletesWithoutSwitchBack(t *testing.T) {
	started := make(chan string, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		started <- "first"
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		// 连续输出 128 个片段，验证隐藏会话无需 UI 消费事件也能一直读取。
		for i := 0; i < 128; i++ {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"background-output \"}}]}\n\n")
			w.(http.Flusher).Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	a := taskTestApp(t, server)
	first := a.CurrentSession
	job, err := startTaskForTest(a, context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "first")
	if err := handleNew(a, nil); err != nil {
		t.Fatal(err)
	}
	second := a.CurrentSession
	m := newTerminal(a, context.Background())
	close(release)
	if err := waitTask(t, job); err != nil {
		t.Fatal(err)
	}
	if a.CurrentSession != second || first.snapshot().Running || first.snapshot().Status != "已完成" {
		t.Fatal("未切回时后台未完成，或意外改变当前会话")
	}
	data, err := os.ReadFile(filepath.Join(a.config.ChatsDir, first.history.SessionID+".jsonl"))
	if err != nil || strings.Count(string(data), "background-output") != 128 {
		t.Fatalf("后台完整结果没有及时落盘：%v", err)
	}
	m = updateTerminal(m, refreshMsg{})
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "会话 1 已完成") || !strings.Contains(view, "/ps 1 查看") {
		t.Fatalf("前台未显示后台完成提示：%s", view)
	}
	if strings.Contains(view, "background-output") {
		t.Fatal("其他会话的正文混入当前会话")
	}
	if err := handleSessions(a, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ansi.Strip(newTerminal(a, context.Background()).View().Content), "background-output") {
		t.Fatal("切回后没有立即显示已经完成的结果")
	}
	if first.snapshot().HasUnreadResult {
		t.Fatal("查看结果后未清除未读提示")
	}
}

// startTaskForTest 通过与主循环相同的入口提交任务，由会话自己的消费循环执行。
func startTaskForTest(a *App, ctx context.Context, query string) (*task, error) {
	return a.enqueueTask(ctx, query)
}

// taskTestApp 创建使用临时记录和本地模拟模型的应用，不读取或保存用户配置。
func taskTestApp(t *testing.T, server *httptest.Server) *App {
	t.Helper()
	cfg := config.Config{ChatsDir: t.TempDir(), ProjectDir: t.TempDir(), BaseURL: server.URL, Model: "initial-model"}
	record, err := startSession(cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{config: cfg}
	a.attachSession(record)
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	return a
}

// waitTask 在有限时间内等待任务退出，避免取消逻辑回归时测试无限等待。
func waitTask(t *testing.T, task *task) error {
	t.Helper()
	select {
	case <-task.done:
		return task.err
	case <-time.After(5 * time.Second):
		t.Fatal("任务未结束")
		return nil
	}
}

// awaitRequest 等待模拟模型收到请求，保证测试的切换确实发生在任务执行期间。
func awaitRequest(t *testing.T, started <-chan string, want string) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("request=%q, want=%q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("模型未收到请求")
	}
}

// TestConcurrentSessionsAndStop 验证两轮任务同时执行、输出隔离、同会话新提问排队、停止只影响当前会话。
func TestConcurrentSessionsAndStop(t *testing.T) {
	started := make(chan string, 2)
	releaseSecond := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		query := request.Messages[len(request.Messages)-1].Content
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", "reply-"+query)
		w.(http.Flusher).Flush()
		started <- query
		if query == "first" {
			<-r.Context().Done()
			return
		}
		select {
		case <-releaseSecond:
			fmt.Fprint(w, "data: [DONE]\n\n")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	a := taskTestApp(t, server)
	first := a.CurrentSession
	firstTask, err := startTaskForTest(a, context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "first")
	queued, err := startTaskForTest(a, context.Background(), "duplicate")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.snapshot().Pending) != 1 {
		t.Fatal("新提问未进入等待队列")
	}
	if err := handleNew(a, nil); err != nil {
		t.Fatal(err)
	}
	second := a.CurrentSession
	secondTask, err := startTaskForTest(a, context.Background(), "second")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "second")
	if !first.snapshot().Running || !second.snapshot().Running {
		t.Fatal("切换时任务被停止")
	}
	if firstTask == secondTask {
		t.Fatal("两次执行错误地共用了同一个任务对象")
	}
	if err := handleSessions(a, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if err := handleResume(a, []string{second.history.SessionID}); err != nil {
		t.Fatal(err)
	}
	if len(a.opened) != 2 || a.CurrentSession != second {
		t.Fatal("恢复已打开会话重复创建了写入者")
	}
	if err := handleSessions(a, []string{"1"}); err != nil {
		t.Fatal(err)
	}
	if err := handleStop(a, nil); err != nil {
		t.Fatal(err)
	}
	if err := waitTask(t, firstTask); !errors.Is(err, context.Canceled) {
		t.Fatalf("first task: %v", err)
	}
	if err := waitTask(t, queued); !errors.Is(err, context.Canceled) {
		t.Fatalf("排队任务未取消：%v", err)
	}
	if !second.snapshot().Running {
		t.Fatal("停止会话 1 影响了会话 2")
	}
	close(releaseSecond)
	if err := waitTask(t, secondTask); err != nil {
		t.Fatal(err)
	}
	if !second.snapshot().HasUnreadResult || second.snapshot().Status != "已完成" {
		t.Fatal("后台完成没有标记新结果")
	}
	firstJSON, _ := json.Marshal(first.history.ToChatCompletionMessage())
	secondJSON, _ := json.Marshal(second.history.ToChatCompletionMessage())
	if strings.Contains(string(firstJSON), "second") || strings.Contains(string(secondJSON), "first") {
		t.Fatal("会话上下文交叉")
	}
	if strings.Contains(string(firstJSON), "duplicate") {
		t.Fatal("未执行的排队任务写入了历史")
	}
	if !strings.Contains(string(secondJSON), "reply-second") {
		t.Fatal("会话 2 结果未保存")
	}
	if err := handleSessions(a, []string{"2"}); err != nil {
		t.Fatal(err)
	}
	if second.snapshot().HasUnreadResult {
		t.Fatal("切回会话后未清除提示")
	}
}

// TestTaskCapturesConfigAndDirectory 验证修改应用配置再切换后，旧任务仍使用启动时的模型、接口、密钥和目录。
func TestTaskCapturesConfigAndDirectory(t *testing.T) {
	started := make(chan string, 1)
	release := make(chan struct{})
	var originalDir string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Model != "initial-model" {
			t.Errorf("运行中的模型变成了 %s", request.Model)
		}
		if r.Header.Get("Authorization") != "Bearer initial-key" {
			t.Error("运行中的任务没有使用启动时的密钥")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		last := request.Messages[len(request.Messages)-1]
		if last.Role == "user" {
			w.(http.Flusher).Flush()
			started <- "old-task"
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"pwd","type":"function","function":{"name":"run_bash","arguments":"{\"command\":\"pwd\"}"}}]},"finish_reason":"tool_calls"}]}`+"\n\n")
		} else {
			if last.Role != "tool" || strings.TrimSpace(last.Content) != originalDir {
				t.Errorf("工具使用了其他工作目录：%+v", last)
			}
			fmt.Fprint(w, "data: "+`{"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}`+"\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	a := taskTestApp(t, server)
	originalDir = a.config.ProjectDir
	a.config.APIKey = "initial-key"
	task, err := startTaskForTest(a, context.Background(), "old-task")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "old-task")
	// /config 修改唯一的应用配置，正在运行的任务不能中途读取这些新值。
	a.config.Model = "changed-model"
	a.config.BaseURL = "http://127.0.0.1:1"
	a.config.APIKey = "changed-key"
	a.config.ProjectDir = t.TempDir()
	if err := handleNew(a, nil); err != nil {
		t.Fatal(err)
	}
	a.config.ProjectDir = t.TempDir()
	a.config.Model = "another-model"
	close(release)
	if err := waitTask(t, task); err != nil {
		t.Fatal(err)
	}
}

// TestAllSessionsUseAppConfig 验证修改应用配置后，切回任意旧会话都会使用新参数，历史文件不迁移。
func TestAllSessionsUseAppConfig(t *testing.T) {
	projectDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		if request.Model != "shared-model" || r.Header.Get("Authorization") != "Bearer shared-key" {
			t.Error("旧会话没有使用统一配置")
		}
		if len(request.Messages) == 0 || !strings.Contains(request.Messages[0].Content, projectDir) {
			t.Error("旧会话的系统提示词没有使用当前工作目录")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	a := taskTestApp(t, server)
	first := a.CurrentSession
	firstPath := first.history.Path()
	oldChatsDir := a.config.ChatsDir
	if err := handleNew(a, nil); err != nil {
		t.Fatal(err)
	}
	second := a.CurrentSession
	a.config.Model = "shared-model"
	a.config.APIKey = "shared-key"
	a.config.ProjectDir = projectDir
	a.config.ChatsDir = t.TempDir()
	for _, session := range []*Session{first, second} {
		a.activateSession(session)
		job, err := a.enqueueTask(context.Background(), "使用新配置")
		if err != nil {
			t.Fatal(err)
		}
		if err := waitTask(t, job); err != nil {
			t.Fatal(err)
		}
	}
	if first.history.Path() != firstPath || a.openedHistory(oldChatsDir, first.history.SessionID) != first {
		t.Fatal("改变配置后无法定位已打开的原记录")
	}
	if a.openedHistory(a.config.ChatsDir, first.history.SessionID) != nil {
		t.Fatal("旧记录被错误识别为新目录中的记录")
	}
}

// TestCloseStopsAllTasks 验证退出会取消并等待全部后台任务，再关闭记录文件。
func TestCloseStopsAllTasks(t *testing.T) {
	started := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		started <- "running"
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	a := taskTestApp(t, server)
	first, err := startTaskForTest(a, context.Background(), "one")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "running")
	if err := handleNew(a, nil); err != nil {
		t.Fatal(err)
	}
	second, err := startTaskForTest(a, context.Background(), "two")
	if err != nil {
		t.Fatal(err)
	}
	awaitRequest(t, started, "running")
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	for _, task := range []*task{first, second} {
		if err := waitTask(t, task); !errors.Is(err, context.Canceled) {
			t.Fatalf("关闭未取消任务：%v", err)
		}
	}
}
