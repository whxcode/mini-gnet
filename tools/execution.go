package tools

import (
	"os"
	"os/exec"
	"time"
)

// ExecutionInfo 记录真实子进程调用，仅用于本地详情和历史，不放进模型的工具结果。
type ExecutionInfo struct {
	Executable string   `json:"executable"`
	Argv       []string `json:"argv"`
	Cwd        string   `json:"cwd"`
	Started    bool     `json:"started"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

// RunProcess 使用已经配置好的进程，记录实际路径、参数、目录和执行状态。
func RunProcess(command *exec.Cmd) (*ExecutionInfo, error) {
	directory := command.Dir
	if directory == "" {
		directory, _ = os.Getwd()
	}

	execution := &ExecutionInfo{
		Executable: command.Path,
		Argv:       append([]string(nil), command.Args...),
		Cwd:        directory,
	}

	startedAt := time.Now()
	err := command.Run()
	execution.DurationMS = time.Since(startedAt).Milliseconds()
	execution.Started = command.Process != nil

	if command.ProcessState != nil {
		code := command.ProcessState.ExitCode()
		execution.ExitCode = &code
	}

	return execution, err
}
