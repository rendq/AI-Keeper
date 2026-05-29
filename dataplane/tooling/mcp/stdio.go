package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

// StdioTransport communicates with an MCP server via stdin/stdout of a subprocess.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mu     sync.Mutex
}

// StdioConfig holds configuration for spawning an MCP server subprocess.
type StdioConfig struct {
	// Command is the executable path.
	Command string
	// Args are the command arguments.
	Args []string
	// Env is optional environment variables for the subprocess.
	Env []string
}

// NewStdioTransport spawns the subprocess and returns a transport that communicates
// with it over stdin/stdout using newline-delimited JSON-RPC 2.0.
func NewStdioTransport(cfg StdioConfig) (*StdioTransport, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	if len(cfg.Env) > 0 {
		cmd.Env = cfg.Env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start process: %w", err)
	}

	return &StdioTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReader(stdout),
	}, nil
}

// Send sends a JSON-RPC 2.0 request over stdin and reads the response from stdout.
func (t *StdioTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("mcp: marshal request: %w", err)
	}

	// Write newline-delimited JSON
	data = append(data, '\n')

	if _, err := t.stdin.Write(data); err != nil {
		return nil, fmt.Errorf("mcp: write stdin: %w", err)
	}

	// Read response line
	type result struct {
		resp *Response
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := t.stdout.ReadBytes('\n')
		if err != nil {
			ch <- result{err: fmt.Errorf("mcp: read stdout: %w", err)}
			return
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			ch <- result{err: fmt.Errorf("mcp: unmarshal response: %w", err)}
			return
		}
		ch <- result{resp: &resp}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.resp, r.err
	}
}

// Close terminates the subprocess and releases resources.
func (t *StdioTransport) Close() error {
	t.stdin.Close()
	return t.cmd.Wait()
}
