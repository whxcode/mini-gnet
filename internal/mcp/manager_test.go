package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// testServer 提供无需联网的有效服务配置。
func testServer(name string) Server {
	return Server{
		Name:        name,
		Description: "测试文档服务",
		Transport:   StreamableHTTP,
		URL:         "https://example.invalid/mcp",
	}
}

// TestManagerLifecycle 验证首次加载、增改查删、重载和调用方副本隔离。
func TestManagerLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "mcp.json")
	manager, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.List()) != 0 {
		t.Fatal("首次应为空列表")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("读取不应创建文件")
	}

	original := testServer("docs")
	if err := manager.Add(original); err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(original); err == nil {
		t.Fatal("重复名称未拒绝")
	}
	if err := manager.Add(testServer("second")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Update("docs", testServer("second")); err == nil {
		t.Fatal("改名冲突未拒绝")
	}

	items := manager.List()
	items[0].Name = "modified-by-caller"
	if _, err := manager.Get("docs"); err != nil {
		t.Fatal("列表没有隔离", err)
	}
	matches := manager.Search("文档")
	if len(matches) != 2 {
		t.Fatal("未匹配中文简介")
	}
	matches[0].URL = "bad"
	if got, _ := manager.Get("docs"); got != original {
		t.Fatal("搜索结果没有隔离")
	}
	if len(manager.Search("DOCS")) != 1 || len(manager.Search("missing")) != 0 {
		t.Fatal("搜索行为错误")
	}

	updated := original
	updated.Name = "renamed"
	updated.URL = "http://localhost:3456/mcp?version=1"
	if err := manager.Update("docs", updated); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get("docs"); err == nil {
		t.Fatal("旧名未移除")
	}
	if err := manager.Update("missing", original); err == nil {
		t.Fatal("未知服务不应被更新")
	}
	if err := manager.Remove("missing"); err == nil {
		t.Fatal("未知服务不应被删除")
	}

	reloaded, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manager.List(), reloaded.List()) {
		t.Fatal("重载结果不一致")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatal("文件权限应为 0600", err)
		}
	}

	if err := manager.Remove("renamed"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove("second"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"servers": []`) {
		t.Fatal("空配置应保存为空数组")
	}
}

// TestOpenRejectsInvalidFiles 验证坏文件及未知格式被保留，而非静默重建。
func TestOpenRejectsInvalidFiles(t *testing.T) {
	valid := testServer("docs")
	duplicate, err := json.Marshal(document{Version: 1, Servers: []Server{valid, valid}})
	if err != nil {
		t.Fatal(err)
	}

	for _, data := range []string{
		"{", "null", `{"version":2,"servers":[]}`,
		`{"version":1,"servers":[],"typo":true}`,
		`{"version":1,"servers":[]} {}`,
		`{"version":1,"servers":[]} trailing`,
		`{"version":1,"servers":[{"name":"docs","transport":"stdio","url":"https://example.com"}]}`,
		string(duplicate),
	} {
		path := filepath.Join(t.TempDir(), "mcp.json")
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Fatalf("错误配置被接受：%s", data)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != data {
			t.Fatal("加载失败修改了文件", err)
		}
	}
}

// TestSaveFailurePreservesMemory 验证落盘失败时增改删均不提交内存状态。
func TestSaveFailurePreservesMemory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	manager, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	original := testServer("docs")
	if err := manager.Add(original); err != nil {
		t.Fatal(err)
	}

	// 用目录占据目标文件路径，稳定触发 Rename 失败，不依赖用户权限。
	if err := os.Rename(path, path+".backup"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := manager.Add(testServer("new")); err == nil {
		t.Fatal("应报告保存失败")
	}
	if err := manager.Update("docs", testServer("renamed")); err == nil {
		t.Fatal("应报告保存失败")
	}
	if err := manager.Remove("docs"); err == nil {
		t.Fatal("应报告保存失败")
	}
	if !reflect.DeepEqual(manager.List(), []Server{original}) {
		t.Fatal("失败保存改变了内存状态")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".mcp-*.json"))
	if err != nil || len(files) != 0 {
		t.Fatal("临时文件未清理", err)
	}
	if data, err := os.ReadFile(filepath.Join(path, "keep")); err != nil || string(data) != "keep" {
		t.Fatal("失败保存破坏原目标")
	}
}

// TestServerValidation 验证名称与地址约束，仅做格式检查。
func TestServerValidation(t *testing.T) {
	for _, name := range []string{"", "../docs", "two words", "-docs", strings.Repeat("x", 65)} {
		if _, err := Normalize(testServer(name)); err == nil {
			t.Fatalf("接受了非法名称 %q", name)
		}
	}
	for _, url := range []string{"", "localhost:80", "ftp://example.com", "https://", "https://user:pass@example.com", "https://example.com/#fragment", "https://example.com:bad"} {
		server := testServer("docs")
		server.URL = url
		if _, err := Normalize(server); err == nil {
			t.Fatalf("接受了非法 URL %q", url)
		}
	}
	server := testServer(" docs ")
	server.URL = " https://example.com/mcp "
	got, err := Normalize(server)
	if err != nil || got.Name != "docs" || got.URL != "https://example.com/mcp" {
		t.Fatal("未清理首尾空格", err)
	}
}

// TestConcurrentSnapshots 验证后台任务读取配置快照时，菜单更新不会发生竞争或修改旧快照。
func TestConcurrentSnapshots(t *testing.T) {
	manager, err := Open(filepath.Join(t.TempDir(), "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Add(testServer("docs")); err != nil {
		t.Fatal(err)
	}
	original, err := manager.Select([]string{"docs"})
	if err != nil {
		t.Fatal(err)
	}

	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for range 100 {
				selected, err := manager.Select([]string{"docs"})
				if err != nil || len(selected) != 1 || selected[0].Name != "docs" {
					t.Error("配置快照不完整", err)
					return
				}
				selected[0].Name = "caller-change"
			}
		})
	}
	for range 5 {
		changed := testServer("docs")
		changed.Description = "修改后的说明"
		if err := manager.Update("docs", changed); err != nil {
			t.Error(err)
		}
	}
	readers.Wait()

	if original[0].Description != "测试文档服务" {
		t.Fatal("运行中的任务快照被菜单更新改变")
	}
}
