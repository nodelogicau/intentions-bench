// Package impl starts an Intentions Format implementation from its command
// template and speaks MCP to it. The harness depends on nothing else an
// implementation provides.
//
// Copied from particulars-bench (internal/impl) at commit
// 770728513dc4f5a84030b047e86903b70662dfc2 and adapted to the intentions
// adapter contract: INTENTIONS_NOW pins the clock, INTENTIONS_HARNESS and the
// MCP client name make every write a harness write, and tool errors keep
// their codes. Fixes made there must be ported by hand.
package impl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Placeholder is replaced by the absolute workspace path in a command.
const Placeholder = "{workspace}"

// Harness is the MCP client name and the INTENTIONS_HARNESS value, so that
// every write the agent makes is attributed to a harness.
const Harness = "intentions-bench"

// Impl is an implementation under test: a label and a command template.
type Impl struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

// Parse reads "<label>=<command>"; the command must contain {workspace}.
func Parse(s string) (Impl, error) {
	label, cmd, ok := strings.Cut(s, "=")
	label, cmd = strings.TrimSpace(label), strings.TrimSpace(cmd)
	if !ok || label == "" || cmd == "" {
		return Impl{}, fmt.Errorf("--impl %q: want <label>=<command>", s)
	}
	if !strings.Contains(cmd, Placeholder) {
		return Impl{}, fmt.Errorf("--impl %s: command must contain %s", label, Placeholder)
	}
	return Impl{Label: label, Command: cmd}, nil
}

// ServerInfo identifies an implementation, from initialize.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// StartError is a server that failed before initialize completed.
type StartError struct {
	Command string
	Err     error
	Stderr  string
}

func (e *StartError) Error() string {
	msg := fmt.Sprintf("starting %q: %v", e.Command, e.Err)
	if s := strings.TrimSpace(e.Stderr); s != "" {
		msg += "\nstderr:\n" + s
	}
	return msg
}

// Server is a running implementation bound to one workspace.
type Server struct {
	Info         ServerInfo
	Instructions string
	Tools        []*mcp.Tool
	// Workspace is the absolute path substituted for {workspace}.
	Workspace string
	session   *mcp.ClientSession
	stderr    *syncBuffer
}

// Start runs impl against workspace: {workspace} becomes the absolute path,
// which is also the working directory; INTENTIONS_NOW is set to now and
// INTENTIONS_HARNESS to Harness. It completes initialize and tools/list.
func Start(ctx context.Context, im Impl, workspace string, now time.Time) (*Server, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	command := strings.ReplaceAll(im.Command, Placeholder, shellQuote(abs))
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = abs
	cmd.Env = append(os.Environ(),
		"INTENTIONS_NOW="+now.Format(time.RFC3339),
		"INTENTIONS_HARNESS="+Harness,
	)
	stderr := &syncBuffer{}
	cmd.Stderr = stderr

	client := mcp.NewClient(&mcp.Implementation{Name: Harness, Version: Version}, nil)
	transport := &mcp.CommandTransport{Command: cmd, TerminateDuration: 2 * time.Second}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		return nil, &StartError{Command: command, Err: err, Stderr: stderr.String()}
	}
	s := &Server{Workspace: abs, session: session, stderr: stderr}
	init := session.InitializeResult()
	if init != nil {
		s.Instructions = init.Instructions
		if init.ServerInfo != nil {
			s.Info = ServerInfo{Name: init.ServerInfo.Name, Version: init.ServerInfo.Version}
		}
	}
	for t, err := range session.Tools(ctx, nil) {
		if err != nil {
			_ = s.Close()
			return nil, &StartError{Command: command, Err: fmt.Errorf("tools/list: %w", err), Stderr: stderr.String()}
		}
		s.Tools = append(s.Tools, t)
	}
	return s, nil
}

// Result is a tool call's outcome: the text the model receives, and for an
// error result the error code the implementation gave.
type Result struct {
	Text    string
	IsError bool
	// Code is the error code: structuredContent.error.code, else a leading
	// "<code>: " in the text. Empty when the result is not an error or
	// carries no code.
	Code string
}

// CallTool forwards a tool call. The text is the text content verbatim, any
// other content as its JSON.
func (s *Server) CallTool(ctx context.Context, name string, args json.RawMessage) (Result, error) {
	var arguments any = map[string]any{}
	if len(args) > 0 {
		arguments = args
	}
	res, err := s.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return Result{IsError: true}, err
	}
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
			continue
		}
		b, err := json.Marshal(c)
		if err != nil {
			return Result{IsError: true}, err
		}
		parts = append(parts, string(b))
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			return Result{IsError: true}, err
		}
		parts = append(parts, string(b))
	}
	out := Result{Text: strings.Join(parts, "\n"), IsError: res.IsError}
	if res.IsError {
		out.Code = errorCode(res.StructuredContent, out.Text)
	}
	return out, nil
}

// Call implements agent.Caller.
func (s *Server) Call(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	r, err := s.CallTool(ctx, name, args)
	return r.Text, r.IsError, err
}

var codePrefix = regexp.MustCompile(`^([a-z][a-z_]*): `)

func errorCode(structured any, text string) string {
	b, err := json.Marshal(structured)
	if err == nil {
		var v struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(b, &v) == nil && v.Error.Code != "" {
			return v.Error.Code
		}
	}
	if m := codePrefix.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// Stderr returns what the server has written to stderr so far.
func (s *Server) Stderr() string { return s.stderr.String() }

// Close ends the session and the server process.
func (s *Server) Close() error { return s.session.Close() }

// Version is the harness version reported as the MCP client.
var Version = "dev"

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// syncBuffer is a bytes.Buffer safe for the process's stderr goroutine,
// keeping only the last 64 KiB.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	if over := b.buf.Len() - 64<<10; over > 0 {
		b.buf.Next(over)
	}
	return n, err
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
