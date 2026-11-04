package external

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/whxcode/mini-gnet/tools"
	"github.com/whxcode/mini-gnet/tools/builtin"
)

// TestExternalProcess 用测试程序自身模拟外部工具，不依赖 Python、网络或模型。
func TestExternalProcess(t *testing.T) {
	separator := -1

	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}

	if separator == -1 {
		return
	}

	mode := os.Args[separator+1]

	switch mode {
	case "echo":
		var input map[string]string
		if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
			os.Exit(2)
		}

		directory, _ := os.Getwd()
		fmt.Fprintln(os.Stderr, "正常日志，不应混入 JSON")
		json.NewEncoder(os.Stdout).Encode(map[string]string{
			"text": input["text"],
			"cwd":  directory,
			"arg":  os.Args[separator+2],
		})

	case "failure":
		fmt.Fprintln(os.Stdout, "未完成的输出")
		fmt.Fprintln(os.Stderr, "外部工具主动失败")
		os.Exit(7)

	case "invalid":
		fmt.Fprintln(os.Stdout, "日志不能放在 stdout")

	case "wrong-schema":
		fmt.Fprintln(os.Stdout, `{"text":42}`)

	case "empty":

	case "large-stdout":
		io.WriteString(os.Stdout, strings.Repeat("x", (1<<20)+1))
		time.Sleep(10 * time.Second)

	case "large-stderr":
		io.WriteString(os.Stderr, strings.Repeat("x", (64<<10)+1))
		time.Sleep(10 * time.Second)

	case "wait":
		time.Sleep(10 * time.Second)
	}

	os.Exit(0)
}

// writeToolFixture 创建真实 tool.json，覆盖加载和执行之间的完整路径。
func writeToolFixture(t *testing.T, root, folder, name, mode string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	config := manifest{
		Definition: tools.Definition{
			SchemaVersion: 1,
			Name:          name,
			Description:   "测试外部程序",
			InputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{"text":{"type":"string"}},
				"required":["text"],
				"additionalProperties":false
			}`),
			OutputSchema: json.RawMessage(`{
				"type":"object",
				"properties":{
					"text":{"type":"string"},
					"cwd":{"type":"string"},
					"arg":{"type":"string"}
				},
				"required":["text","cwd","arg"],
				"additionalProperties":false
			}`),
		},
		Execution: execution{
			Command: executable,
			Args: []string{
				"-test.run=^TestExternalProcess$",
				"--",
				mode,
				"带空格参数 ; $(不会执行)",
			},
			Cwd: ".",
		},
	}

	directory := filepath.Join(root, folder)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(directory, "tool.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// TestProcessProtocol 验证 JSON、中文、独立 cwd 和字面参数不经过 Shell 展开。
func TestProcessProtocol(t *testing.T) {
	root := t.TempDir()
	path := writeToolFixture(t, root, "tool with spaces", "echo", "echo")
	implementations, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	registry, err := tools.NewRegistry(implementations...)
	if err != nil {
		t.Fatal(err)
	}

	before, _ := os.Getwd()
	result := registry.Execute(context.Background(), "echo", tools.Request{
		Arguments: json.RawMessage(`{"text":"你好，成都"}`),
		Context:   tools.Context{WorkDir: t.TempDir()},
	})
	if !result.OK {
		t.Fatalf("执行失败：%+v", result.Error)
	}

	if result.Execution == nil ||
		!result.Execution.Started ||
		result.Execution.ExitCode == nil ||
		*result.Execution.ExitCode != 0 ||
		result.Execution.Argv[len(result.Execution.Argv)-1] != "带空格参数 ; $(不会执行)" {
		t.Fatalf("外部命令信息没有保留真实参数：%+v", result.Execution)
	}

	var data map[string]string
	if err := json.Unmarshal(result.Data.(json.RawMessage), &data); err != nil {
		t.Fatal(err)
	}

	wantDirectory, _ := filepath.EvalSymlinks(filepath.Dir(path))
	gotDirectory, _ := filepath.EvalSymlinks(data["cwd"])
	after, _ := os.Getwd()

	if data["text"] != "你好，成都" ||
		data["arg"] != "带空格参数 ; $(不会执行)" ||
		gotDirectory != wantDirectory ||
		after != before {
		t.Fatalf("输入输出或目录发生变化：%v", data)
	}

	declarations, _ := json.Marshal(registry.ModelTools())
	if strings.Contains(string(declarations), "execution") {
		t.Fatal("启动配置不应发送给模型")
	}

	invalid := registry.Execute(context.Background(), "echo", tools.Request{
		Arguments: json.RawMessage(`{"text":42}`),
	})
	if invalid.Error == nil || invalid.Error.Code != tools.InvalidArguments {
		t.Fatal("错误输入应在启动进程之前被拒绝")
	}
}

// TestExecutionPaths 验证相对可执行路径基于定义目录，而 cwd 可指向上层或绝对目录。
func TestExecutionPaths(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}

	path := writeToolFixture(t, root, "tool", "echo", "echo")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var config manifest
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	config.Execution.Command, err = filepath.Rel(filepath.Dir(path), config.Execution.Command)
	if err != nil {
		t.Fatal(err)
	}

	for _, directory := range []string{"../shared", shared} {
		config.Execution.Cwd = directory
		data, err = json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}

		implementations, err := LoadDir(root)
		if err != nil {
			t.Fatal(err)
		}

		registry, err := tools.NewRegistry(implementations...)
		if err != nil {
			t.Fatal(err)
		}

		result := registry.Execute(context.Background(), "echo", tools.Request{
			Arguments: json.RawMessage(`{"text":"hello"}`),
		})
		if !result.OK {
			t.Fatalf("相对命令执行失败：%+v", result.Error)
		}

		var output map[string]string
		if err := json.Unmarshal(result.Data.(json.RawMessage), &output); err != nil {
			t.Fatal(err)
		}

		got, _ := filepath.EvalSymlinks(output["cwd"])
		want, _ := filepath.EvalSymlinks(shared)
		if got != want {
			t.Fatalf("cwd 没有按配置生效：%s", output["cwd"])
		}
	}
}

// TestMissingExecutableAndCancellation 验证解释器缺失与任务取消有明确的不同错误码。
func TestMissingExecutableAndCancellation(t *testing.T) {
	root := t.TempDir()
	writeToolFixture(t, root, "tool", "echo", "echo")
	implementations, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	implementations[0].(*ProcessTool).command = filepath.Join(root, "missing-executable")
	registry, err := tools.NewRegistry(implementations...)
	if err != nil {
		t.Fatal(err)
	}

	request := tools.Request{Arguments: json.RawMessage(`{"text":"hello"}`)}
	result := registry.Execute(context.Background(), "echo", request)
	if result.Error == nil || result.Error.Code != tools.ExecutionFailed {
		t.Fatalf("缺少解释器应报告执行错误：%+v", result.Error)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = registry.Execute(ctx, "echo", request)
	if result.Error == nil || result.Error.Code != tools.Canceled {
		t.Fatalf("取消应有独立错误码：%+v", result.Error)
	}
}

// TestProcessFailures 验证失败退出、无效输出、超限和取消统一返回错误代码。
func TestProcessFailures(t *testing.T) {
	cases := []struct {
		mode string
		code string
	}{
		{mode: "failure", code: tools.ExecutionFailed},
		{mode: "invalid", code: tools.InvalidOutput},
		{mode: "wrong-schema", code: tools.InvalidOutput},
		{mode: "empty", code: tools.InvalidOutput},
		{mode: "large-stdout", code: tools.InvalidOutput},
		{mode: "large-stderr", code: tools.InvalidOutput},
		{mode: "wait", code: tools.Timeout},
	}

	for _, test := range cases {
		t.Run(test.mode, func(t *testing.T) {
			root := t.TempDir()
			writeToolFixture(t, root, "tool", "test_tool", test.mode)
			implementations, err := LoadDir(root)
			if err != nil {
				t.Fatal(err)
			}

			registry, err := tools.NewRegistry(implementations...)
			if err != nil {
				t.Fatal(err)
			}

			timeout := 5 * time.Second
			if test.mode == "wait" {
				timeout = 150 * time.Millisecond
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			result := registry.Execute(ctx, "test_tool", tools.Request{
				Arguments: json.RawMessage(`{"text":"hello"}`),
			})
			if result.OK || result.Error == nil || result.Error.Code != test.code {
				t.Fatalf("错误代码应为 %s：%+v", test.code, result)
			}

			if result.Execution == nil {
				t.Fatal("执行失败或输出校验失败后丢失了进程信息")
			}

			if test.mode == "failure" && !strings.Contains(result.Error.Message, "外部工具主动失败") {
				t.Fatal("错误信息未包含 stderr")
			}
		})
	}
}

// TestLoadValidation 验证缺失目录可启动，错误定义和同名工具不会被静默接受。
func TestLoadValidation(t *testing.T) {
	root := t.TempDir()
	if loaded, err := LoadDir(filepath.Join(root, "missing")); err != nil || len(loaded) != 0 {
		t.Fatalf("不存在的目录应为空：%v", err)
	}

	path := writeToolFixture(t, root, "tool", "run_python", "echo")
	loaded, err := LoadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	implementations, err := builtin.Load()
	if err != nil {
		t.Fatal(err)
	}

	implementations = append(implementations, loaded...)
	if _, err := tools.NewRegistry(implementations...); err == nil {
		t.Fatal("不应覆盖同名内置工具")
	}

	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, invalid := range []string{
		"{",
		string(original) + " {}",
		strings.Replace(string(original), `"execution":`, `"exec_typo":`, 1),
		strings.Replace(string(original), `"cwd": "."`, `"cwd": "missing"`, 1),
	} {
		if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := LoadDir(root); err == nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("错误定义应携带文件路径：%v", err)
		}
	}
}
