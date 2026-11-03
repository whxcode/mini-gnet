package session_test

import (
	"testing"
	"time"

	"github.com/whxcode/mini-gnet/config"
	"github.com/whxcode/mini-gnet/internal/agent"
	sessions "github.com/whxcode/mini-gnet/internal/session"
	tasks "github.com/whxcode/mini-gnet/internal/task"
	"github.com/whxcode/mini-gnet/tools/builtin"
)

// sessionForTest 创建一个带真实历史文件的会话，不经过 App 或终端界面。
func sessionForTest(t *testing.T, url string) (*config.Store, *sessions.Session) {
	t.Helper()
	cfg := config.Config{
		ChatsDir:   t.TempDir(),
		ProjectDir: t.TempDir(),
		BaseURL:    url,
		Model:      "initial-model",
	}
	store := config.NewStore(cfg)
	history, err := sessions.NewHistory(cfg)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	s := sessions.New(history, agent.New(store, registry))
	go s.Run()
	t.Cleanup(func() {
		s.Shutdown()
		s.Close()
	})

	return store, s
}

// waitTask 等待公开的完成通知，防止队列回归时测试无限等待。
func waitTask(t *testing.T, job *tasks.Task) error {
	t.Helper()

	select {
	case <-job.Done:
		return job.Err
	case <-time.After(5 * time.Second):
		t.Fatal("任务未结束")

		return nil
	}
}

// awaitRequest 确认模拟服务收到请求后再进行下一步切换或取消。
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
