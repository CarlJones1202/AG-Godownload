package services

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// testPython returns a working Python interpreter for live semantic-worker
// tests: EMBED_PYTHON first (as configured in .env), then the repo-local
// embedding venv, then PATH. Skips when none is usable — the Microsoft Store
// python.exe stub is on PATH but is not a real interpreter.
func testPython(t *testing.T) string {
	t.Helper()
	candidates := []string{
		os.Getenv("EMBED_PYTHON"),
		filepath.Join("..", "bin", "embedding-venv", "Scripts", "python.exe"),
		"python3",
		"python",
	}
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		p, err := exec.LookPath(cand)
		if err != nil {
			continue
		}
		if err := exec.Command(p, "--version").Run(); err == nil {
			return p
		}
	}
	t.Skip("no working Python interpreter found (set EMBED_PYTHON)")
	return ""
}

// TestSemanticWorkerCommandForcesUTF8 guards the fix for cp1252 stdin decoding:
// the Python child must be forced into UTF-8 mode or it mangles/rejects
// non-ASCII image paths sent by the Go side.
func TestSemanticWorkerCommandForcesUTF8(t *testing.T) {
	cmd := newSemanticWorkerCommand("python", "script.py", "model.onnx")

	found := false
	for _, kv := range cmd.Env {
		if kv == "PYTHONUTF8=1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("PYTHONUTF8=1 missing from semantic worker environment")
	}
	if cmd.Stderr != os.Stderr {
		t.Error("semantic worker stderr should be inherited from the parent")
	}
}

// TestSemanticEmbedderScriptIgnoresSIGINT guards the fix for the KeyboardInterrupt
// traceback printed during API shutdown: on Windows Ctrl+C hits every process on
// the console, so the child must ignore it and let the Go parent end it.
func TestSemanticEmbedderScriptIgnoresSIGINT(t *testing.T) {
	if !strings.Contains(string(semanticEmbedderScript), "signal.signal(signal.SIGINT, signal.SIG_IGN)") {
		t.Error("embedded semantic_embedder.py must ignore SIGINT (signal.signal(signal.SIGINT, signal.SIG_IGN))")
	}
}

// TestSemanticWorkerUTF8PathRoundTrip spawns a real Python child through the
// same command builder the semantic worker uses and round-trips a non-ASCII
// image path over stdin/stdout. Proves the UTF-8 fix end to end at the process
// level. Requires Python; skipped with -short.
func TestSemanticWorkerUTF8PathRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a Python process; skipped with -short")
	}
	python := testPython(t)

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "echo_worker.py")
	script := "import json, sys\n" +
		"for line in sys.stdin:\n" +
		"    req = json.loads(line)\n" +
		"    print(json.dumps({'path': req['path']}), flush=True)\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cmd := newSemanticWorkerCommand(python, scriptPath, "")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start python: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	want := "café/東京/naïve photo.jpg"
	if err := json.NewEncoder(stdin).Encode(map[string]string{"path": want}); err != nil {
		t.Fatalf("send path: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var resp struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("decode response %q: %v", line, err)
	}
	if resp.Path != want {
		t.Errorf("path corrupted in transit through the child: sent %q, got %q", want, resp.Path)
	}
}
