package builtin

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whxcode/mini-gnet/tools"
)

// requestForTest 用真实 JSON 构造模型参数，程序另行注入默认目录。
func requestForTest(t *testing.T, dir string, arguments any) tools.Request {
	t.Helper()
	data, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}

	return tools.Request{
		Arguments: data,
		Context:   tools.Context{WorkDir: dir},
	}
}

// TestBuiltinsOutsideRepository 验证从空目录加载嵌入定义、默认/上层 cwd 和项目外绝对路径读取。
func TestBuiltinsOutsideRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	if len(registry.ModelTools()) != 2 {
		t.Fatal("内置工具未完整注册")
	}

	root := t.TempDir()
	project := filepath.Join(root, "project")

	if err := os.Mkdir(project, 0o700); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(root, "outside.txt")

	if err := os.WriteFile(file, []byte("outside-project"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		args map[string]string
		want string
	}{
		{
			map[string]string{"command": "pwd"},
			project,
		},
		{
			map[string]string{
				"command": "pwd",
				"cwd":     "..",
			},
			root,
		},
		{
			map[string]string{
				"command": "pwd",
				"cwd":     root,
			},
			root,
		},
		{
			map[string]string{"command": "cat '" + file + "'"},
			"outside-project",
		},
		{
			map[string]string{"command": "cat ../outside.txt"},
			"outside-project",
		},
	} {
		result := registry.Execute(context.Background(), "run_bash", requestForTest(t, project, tc.args))
		output, ok := result.Data.(tools.ProcessOutput)
		if !result.OK || !ok || output.ExitCode != 0 {
			t.Fatalf("执行失败：%+v", result)
		}

		got := strings.TrimSpace(output.Stdout)
		want := tc.want

		// macOS 的 /var 与 /private/var 指向同一目录，pwd 可能输出物理路径。
		if tc.args["command"] == "pwd" {
			got, err = filepath.EvalSymlinks(got)
			if err != nil {
				t.Fatal(err)
			}

			want, err = filepath.EvalSymlinks(want)
			if err != nil {
				t.Fatal(err)
			}
		}

		if got != want {
			t.Fatalf("got=%q, want=%q", got, want)
		}
	}

	before, _ := os.Getwd()
	registry.Execute(context.Background(), "run_bash", requestForTest(t, project, map[string]string{
		"command": "pwd",
		"cwd":     "..",
	}))
	after, _ := os.Getwd()

	if before != after {
		t.Fatal("工具改变了主进程目录")
	}
}

// TestBuiltinErrors 验证参数错误、目录错误、命令错误和取消都使用统一结果。
func TestBuiltinErrors(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	for _, args := range []string{
		`{}`,
		`null`,
		`[]`,
		`{"command":1}`,
		`{"command":"pwd","extra":1}`,
		`{"command":"pwd","cwd":"missing"}`,
		`{"command":"pwd\nls"}`,
	} {
		result := registry.Execute(context.Background(), "run_bash", tools.Request{
			Arguments: json.RawMessage(args),
			Context:   tools.Context{WorkDir: dir},
		})

		if result.OK || result.Error == nil || result.Error.Code != tools.InvalidArguments {
			t.Fatalf("参数 %s: %+v", args, result)
		}
	}

	failed := registry.Execute(
		context.Background(),
		"run_bash",
		requestForTest(t, dir, map[string]string{"command": "cat missing-file"}),
	)

	if failed.OK ||
		failed.Error.Code != tools.ExecutionFailed ||
		failed.Data.(tools.ProcessOutput).Stderr == "" {
		t.Fatalf("未保留失败输出：%+v", failed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := registry.Execute(ctx, "run_bash", requestForTest(t, dir, map[string]string{"command": "pwd"}))

	if canceled.OK || canceled.Error.Code != tools.Canceled {
		t.Fatalf("取消结果错误：%+v", canceled)
	}
}

// TestPythonTool 验证 Python 同样通过注册表执行，成功与失败保留相同的数据结构。
func TestPythonTool(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}

	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}

	for _, code := range []string{"print(1 + 2)", "print('partial')\nraise RuntimeError('test failure')"} {
		result := registry.Execute(
			context.Background(),
			"run_python",
			requestForTest(t, t.TempDir(), map[string]string{"code": code}),
		)
		output, ok := result.Data.(tools.ProcessOutput)
		if !ok {
			t.Fatalf("缺少 Python 输出：%+v", result)
		}

		if strings.Contains(code, "raise") {
			if result.OK ||
				result.Error == nil ||
				output.ExitCode == 0 ||
				!strings.Contains(output.Stderr, "test failure") ||
				!strings.Contains(output.Stdout, "partial") {
				t.Fatalf("失败结果：%+v", result)
			}
		} else if !result.OK || strings.TrimSpace(output.Stdout) != "3" || output.ExitCode != 0 {
			t.Fatalf("成功结果：%+v", result)
		}
	}
}
