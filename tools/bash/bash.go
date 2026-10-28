package bash

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

var allowed = map[string]bool{
	"cat":  true,
	"sed":  true,
	"rg":   true,
	"git":  true,
	"go":   true,
	"pwd":  true,
	"ls":   true,
	"find": true,
	"set":  true,
}

// Run executes the model's read-only project command in the configured directory.
// The command is deliberately checked before it reaches the shell.
func Run(ctx context.Context, projectDir, command string) (string, error) {
	if err := validate(command); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "/bin/bash", "-lc", command)
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("command failed: %w", err)
	}

	return string(out), nil
}

func validate(command string) error {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return fmt.Errorf("empty bash command")
	}

	// First version: commands are for inspection and tests only. Chaining is
	// rejected so a permitted command cannot hide a second command.
	if strings.ContainsAny(trimmed, ";|&><`$()") {
		return fmt.Errorf("command contains unsupported shell syntax")
	}

	fields := strings.Fields(trimmed)

	if len(fields) == 0 {
		return fmt.Errorf("empty bash command")
	}

	if !allowed[fields[0]] {
		return fmt.Errorf("command %q is not allowed", fields[0])
	}

	return nil
}
