package run

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requirePython(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
}

func TestPython(t *testing.T) {
	requirePython(t)
	dir := t.TempDir()
	for name, code := range map[string]string{
		"main.py":   "from helper import add\nprint(add(1, 2))\n",
		"helper.py": "def add(a, b):\n    return a + b\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(code), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := Python(dir, "main.py")
	if err != nil || r.ExitCode != 0 || strings.TrimSpace(r.Stdout) != "3" {
		t.Fatalf("result=%+v, err=%v", r, err)
	}
}

func TestPythonCodeCleanup(t *testing.T) {
	requirePython(t)
	for _, fail := range []bool{false, true} {
		code := "import os\nprint(os.getcwd())\nopen('generated.txt', 'w').write('hello')\n"
		if fail {
			code += "raise RuntimeError('test failure')\n"
		}
		r, err := PythonCode(context.Background(), code)
		if fail {
			if err == nil || r.ExitCode == 0 || !strings.Contains(r.Stderr, "test failure") {
				t.Fatalf("result=%+v, err=%v", r, err)
			}
		} else if err != nil || r.ExitCode != 0 {
			t.Fatalf("result=%+v, err=%v", r, err)
		}
		dir := strings.TrimSpace(r.Stdout)
		if !strings.HasPrefix(filepath.Base(dir), "mini-agent-python-") {
			t.Fatalf("unexpected execution directory: %q", dir)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("temp directory was not removed: %v", err)
		}
	}
}

func TestPythonCodeInvalidAndCancelled(t *testing.T) {
	if r, err := PythonCode(context.Background(), " \n"); err == nil || r.ExitCode != -1 {
		t.Fatalf("empty code: result=%+v, err=%v", r, err)
	}
	requirePython(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PythonCode(ctx, "print(1)"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
