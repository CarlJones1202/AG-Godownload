package services

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

//go:embed semantic_embedder.py
var semanticEmbedderScript []byte

type semanticWorkerResponse struct {
	Ready     bool      `json:"ready"`
	Dimension int       `json:"dimension"`
	Vector    []float32 `json:"vector"`
	Error     string    `json:"error"`
}

type semanticInferenceWorker struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	script string
	closed bool
}

// newSemanticWorkerCommand builds the Python child process for the semantic
// worker. PYTHONUTF8=1 forces UTF-8 text I/O on the child: the Go side writes
// UTF-8 JSON paths, but Python otherwise decodes stdin with the locale codec
// (cp1252 on Windows), which mangles non-ASCII paths and outright rejects
// byte sequences that are undefined in cp1252.
func newSemanticWorkerCommand(pythonPath, scriptPath, modelPath string) *exec.Cmd {
	cmd := exec.Command(pythonPath, "-u", scriptPath, modelPath)
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1")
	return cmd
}

func startSemanticInferenceWorker(pythonPath, modelPath string, expectedDimension int) (*semanticInferenceWorker, error) {
	script, err := os.CreateTemp("", "ag-godownload-semantic-*.py")
	if err != nil {
		return nil, fmt.Errorf("create semantic worker script: %w", err)
	}
	scriptPath := script.Name()
	if _, err := script.Write(semanticEmbedderScript); err != nil {
		script.Close()
		os.Remove(scriptPath)
		return nil, fmt.Errorf("write semantic worker script: %w", err)
	}
	if err := script.Close(); err != nil {
		os.Remove(scriptPath)
		return nil, fmt.Errorf("close semantic worker script: %w", err)
	}

	cmd := newSemanticWorkerCommand(pythonPath, scriptPath, modelPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		os.Remove(scriptPath)
		return nil, fmt.Errorf("open semantic worker input: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		os.Remove(scriptPath)
		return nil, fmt.Errorf("open semantic worker output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		os.Remove(scriptPath)
		return nil, fmt.Errorf("start semantic worker: %w", err)
	}

	worker := &semanticInferenceWorker{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
		script: scriptPath,
	}
	type readyResult struct {
		response semanticWorkerResponse
		err      error
	}
	ready := make(chan readyResult, 1)
	go func() {
		response, err := worker.readResponse()
		ready <- readyResult{response: response, err: err}
	}()

	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	select {
	case result := <-ready:
		if result.err != nil {
			worker.Close()
			return nil, fmt.Errorf("initialize semantic worker: %w", result.err)
		}
		if result.response.Error != "" {
			worker.Close()
			return nil, fmt.Errorf("initialize semantic model: %s", result.response.Error)
		}
		if !result.response.Ready || result.response.Dimension != expectedDimension {
			worker.Close()
			return nil, fmt.Errorf("semantic model reported dimension %d (expected %d)",
				result.response.Dimension, expectedDimension)
		}
	case <-timer.C:
		worker.Close()
		return nil, fmt.Errorf("timed out initializing semantic model")
	}
	return worker, nil
}

func (w *semanticInferenceWorker) Embed(path string) ([]float32, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, fmt.Errorf("semantic worker is closed")
	}

	if err := json.NewEncoder(w.stdin).Encode(struct {
		Path string `json:"path"`
	}{Path: path}); err != nil {
		return nil, fmt.Errorf("send image to semantic worker: %w", err)
	}
	response, err := w.readResponse()
	if err != nil {
		return nil, fmt.Errorf("read semantic worker response: %w", err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("semantic inference failed: %s", response.Error)
	}
	return response.Vector, nil
}

func (w *semanticInferenceWorker) readResponse() (semanticWorkerResponse, error) {
	var response semanticWorkerResponse
	line, err := w.stdout.ReadBytes('\n')
	if err != nil {
		return response, err
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return response, fmt.Errorf("decode worker response: %w", err)
	}
	return response, nil
}

func (w *semanticInferenceWorker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	_ = w.stdin.Close()
	if w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
		_ = w.cmd.Wait()
	}
	_ = os.Remove(w.script)
}
