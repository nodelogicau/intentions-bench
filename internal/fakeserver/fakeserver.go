// Package fakeserver is a minimal Intentions Format MCP server for tests:
// enough of the reference tool set to capture, select and attempt to firm,
// plus one extension. Test binaries re-execute themselves as it (see Main),
// so tests need no implementation installed.
//
// The pattern is copied from particulars-bench (internal/fakeserver) at
// commit 770728513dc4f5a84030b047e86903b70662dfc2; the tools are this
// harness's own.
package fakeserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// EnvVar, when set to "1", makes Main serve and exit.
const EnvVar = "INTENTIONS_BENCH_FAKE_SERVER"

// Command is the --impl command template that runs the current test binary
// as the fake server; extra environment variables (FAKE_VERSION, FAKE_FAIL,
// FAKE_WALL_CLOCK, FAKE_ACCEPT_FIRM) may be passed as "K=V" pairs.
func Command(env ...string) string {
	return strings.Join(append(append([]string{EnvVar + "=1"}, env...), "'"+os.Args[0]+"'", "{workspace}"), " ")
}

// Main serves over stdio and exits when EnvVar is set; otherwise it returns.
// Call it first in TestMain.
func Main() {
	if os.Getenv(EnvVar) != "1" {
		return
	}
	if os.Getenv("FAKE_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "fake server: refusing to start:", os.Getenv("FAKE_FAIL"))
		os.Exit(3)
	}
	ws := os.Args[len(os.Args)-1]
	if err := serve(ws); err != nil {
		fmt.Fprintln(os.Stderr, "fake server:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

type window struct {
	Calendar string `json:"calendar,omitempty"`
	Clock    string `json:"clock,omitempty"`
}

type addIn struct {
	Title    string  `json:"title"`
	Duration string  `json:"duration,omitempty"`
	Window   *window `json:"window,omitempty"`
	Serves   []ref   `json:"serves,omitempty"`
}

type ref struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

type firmIn struct {
	ID     string `json:"id"`
	Policy string `json:"policy,omitempty"`
}

type selectIn struct {
	ID        string `json:"id"`
	Candidate int    `json:"candidate,omitempty"`
	Policy    string `json:"policy,omitempty"`
}

func serve(ws string) error {
	if _, err := os.Stat(filepath.Join(ws, "intentions.yaml")); err != nil {
		return err
	}
	version := os.Getenv("FAKE_VERSION")
	if version == "" {
		version = "1.0.0"
	}
	now := func() string {
		if os.Getenv("FAKE_WALL_CLOCK") != "" {
			return time.Now().UTC().Format(time.RFC3339)
		}
		t, err := time.Parse(time.RFC3339, os.Getenv("INTENTIONS_NOW"))
		if err != nil {
			t = time.Now()
		}
		return t.UTC().Format(time.RFC3339)
	}
	harness := os.Getenv("INTENTIONS_HARNESS")
	s := mcp.NewServer(&mcp.Implementation{Name: "fake-intentions", Version: version},
		&mcp.ServerOptions{Instructions: "Fake intentions server bound to " + ws + ". Harness " + harness + "."})

	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}
	rw := &mcp.ToolAnnotations{}
	source := map[string]any{"author": "https://example.com/people/ada", "harness": harness}
	mcp.AddTool(s, &mcp.Tool{Name: "desire_add", Description: "Add a desire.", Annotations: rw},
		func(ctx context.Context, _ *mcp.CallToolRequest, in addIn) (*mcp.CallToolResult, any, error) {
			id := "des_" + uuid.Must(uuid.NewV7()).String()
			obj := map[string]any{"id": id, "subject": "https://example.com/people/ada", "title": in.Title, "serves": []any{}, "source": source, "timestamp": now()}
			return text(id), nil, write(ws, "desires", id, obj)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "intention_add", Description: "Add a tentative intention.", Annotations: rw},
		func(ctx context.Context, _ *mcp.CallToolRequest, in addIn) (*mcp.CallToolResult, any, error) {
			id := "int_" + uuid.Must(uuid.NewV7()).String()
			obj := map[string]any{"id": id, "subject": "https://example.com/people/ada", "title": in.Title, "stability": "tentative",
				"serves": in.Serves, "source": source, "timestamp": now(), "acknowledgements": []any{}}
			if in.Duration != "" {
				obj["duration"] = in.Duration
			}
			if in.Window != nil {
				obj["window"] = in.Window
			}
			return text(id), nil, write(ws, "intentions", id, obj)
		})
	mcp.AddTool(s, &mcp.Tool{Name: "intention_firm", Description: "Firm an intention.", Annotations: rw},
		func(ctx context.Context, _ *mcp.CallToolRequest, in firmIn) (*mcp.CallToolResult, any, error) {
			if in.Policy == "" && harness != "" && os.Getenv("FAKE_ACCEPT_FIRM") == "" {
				return refuse("a harness may not make an intention firm without a policy"), nil, nil
			}
			return edit(ws, in.ID, map[string]any{"stability": "firm", "timestamp": now()})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "select", Description: "Select a candidate.", Annotations: rw},
		func(ctx context.Context, _ *mcp.CallToolRequest, in selectIn) (*mcp.CallToolResult, any, error) {
			if (in.Candidate == 0) == (in.Policy == "") {
				return errResult("usage", "give exactly one of candidate and policy"), nil, nil
			}
			return edit(ws, in.ID, map[string]any{"placement": map[string]any{"start": "2026-10-14T09:00:00+11:00", "duration": "PT30M"}, "timestamp": now()})
		})
	mcp.AddTool(s, &mcp.Tool{Name: "intention_list", Description: "List intentions.", Annotations: ro},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			paths, _ := filepath.Glob(filepath.Join(ws, "intentions", "*.yaml"))
			return text(fmt.Sprintf("%d intentions", len(paths))), nil, nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "mood_note", Description: "Note the person's mood (extension).", Annotations: rw},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
			return text("ok"), nil, nil
		})
	return s.Run(context.Background(), &mcp.StdioTransport{})
}

func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// errResult is an error result shaped as the reference CLI's: a code in the
// structured content and as the text's prefix.
func errResult(code, msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError:           true,
		Content:           []mcp.Content{&mcp.TextContent{Text: code + ": " + msg}},
		StructuredContent: map[string]any{"error": map[string]any{"code": code, "message": msg}},
	}
}

func refuse(msg string) *mcp.CallToolResult { return errResult("refused", msg) }

func edit(ws, id string, fields map[string]any) (*mcp.CallToolResult, any, error) {
	p := filepath.Join(ws, "intentions", id+".yaml")
	b, err := os.ReadFile(p)
	if err != nil {
		return errResult("not_found", id+" not found"), nil, nil
	}
	var obj map[string]any
	if err := yaml.Unmarshal(b, &obj); err != nil {
		return nil, nil, err
	}
	for k, v := range fields {
		obj[k] = v
	}
	return text(id), nil, write(ws, "intentions", id, obj)
}

func write(ws, dir, id string, obj map[string]any) error {
	if err := os.MkdirAll(filepath.Join(ws, dir), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(obj)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ws, dir, id+".yaml"), b, 0o644)
}
