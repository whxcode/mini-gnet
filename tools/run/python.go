package run

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/whxcode/mini-gnet/tools"
)

// RunResult 保留现有调用名称，两个进程工具使用相同的输出数据结构。
type RunResult = tools.ProcessOutput

// Python 手动运行已有脚本，projectDir 只指定子进程的当前目录。
func Python(projectDir, filename string) (RunResult, error) {
	return pythonFile(context.Background(), projectDir, filename)
}

// PythonCode runs a complete script in a fresh temporary directory.
// The directory is removed after execution; it is not a security sandbox.
func PythonCode(ctx context.Context, code string) (RunResult, error) {
	if strings.TrimSpace(code) == "" {
		return RunResult{ExitCode: -1}, fmt.Errorf("python code must not be empty")
	}

	dir, err := os.MkdirTemp("", "mini-agent-python-")
	if err != nil {
		return RunResult{ExitCode: -1}, fmt.Errorf("create python temp directory: %w", err)
	}

	defer os.RemoveAll(dir)

	if err := os.WriteFile(filepath.Join(dir, "main.py"), []byte(code), 0o600); err != nil {
		return RunResult{ExitCode: -1}, fmt.Errorf("write python script: %w", err)
	}

	return pythonFile(ctx, dir, "main.py")
}

// pythonFile 启动解释器，支持超时与取消，并完整收集进程输出。
func pythonFile(ctx context.Context, projectDir, filename string) (RunResult, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute*5)

	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", filename)
	cmd.Dir = projectDir
	cmd.WaitDelay = time.Second

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: -1,
	}

	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctx.Err() != nil {
		return result, ctx.Err()
	}

	return result, err
}
