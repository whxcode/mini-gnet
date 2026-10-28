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
)

type RunResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

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
