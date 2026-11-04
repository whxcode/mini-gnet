package external_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/tools/external"
)

// TestWebSearchNodeProtocol 用真实 Node 脚本和固定搜索响应验证完整调用，不访问公网。
func TestWebSearchNodeProtocol(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}

	source, err := filepath.Abs("../../my_tools/web_search")
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(source, "tool.json"))
	if err != nil {
		t.Fatal(err)
	}

	var definition map[string]any
	if err := json.Unmarshal(data, &definition); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	directory := filepath.Join(root, "web_search")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}

	// 只在测试进程预加载固定响应，生产脚本仍访问它自己的搜索入口。
	preload := `
globalThis.fetch = async (url) => {
  if (url.searchParams.get("q") !== "Go 中文") {
    throw new Error("搜索词未完整传递");
  }

  await new Promise((resolve) => setTimeout(resolve, 25));

  return new Response("<rss><channel><item><title>Go 文档</title><link>https://go.dev/doc/</link><description>学习 Go</description></item></channel></rss>");
};
`
	preloadPath := filepath.Join(directory, "fixture.mjs")
	if err := os.WriteFile(preloadPath, []byte(preload), 0o600); err != nil {
		t.Fatal(err)
	}

	definition["execution"] = map[string]any{
		"command": "node",
		"args":    []string{"--import", preloadPath, filepath.Join(source, "main.mjs")},
		"cwd":     ".",
	}
	data, err = json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, "tool.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	implementations, err := external.LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := tools.NewRegistry(implementations...)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := registry.Execute(ctx, "web_search", tools.Request{
		Arguments: json.RawMessage(`{"query":"Go 中文","limit":1}`),
	})
	if !result.OK {
		t.Fatalf("搜索调用失败：%+v", result.Error)
	}

	var output struct {
		Count   int `json:"count"`
		Results []struct {
			Title string `json:"title"`
			URL   string `json:"url"`
		} `json:"results"`
	}
	if err := json.Unmarshal(result.Data.(json.RawMessage), &output); err != nil {
		t.Fatal(err)
	}

	if output.Count != 1 || len(output.Results) != 1 ||
		output.Results[0].Title != "Go 文档" || output.Results[0].URL != "https://go.dev/doc/" {
		t.Fatalf("搜索数组结果未正确解析：%+v", output)
	}

	if result.Execution == nil || !result.Execution.Started || result.Execution.ExitCode == nil || *result.Execution.ExitCode != 0 {
		t.Fatal("缺少 Node 的实际执行信息")
	}

	invalid := registry.Execute(ctx, "web_search", tools.Request{
		Arguments: json.RawMessage(`{"query":"Go 中文","limit":0}`),
	})
	if invalid.OK || invalid.Error == nil || !strings.Contains(invalid.Error.Message, "limit") {
		t.Fatal("脚本参数错误未通过 stderr 返回")
	}
}
