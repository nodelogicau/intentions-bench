package impl_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/fakeserver"
	"github.com/nodelogicau/intentions-bench/internal/impl"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/testutil"
	"github.com/nodelogicau/intentions-bench/internal/workspace"
)

func TestMain(m *testing.M) {
	fakeserver.Main()
	os.Exit(m.Run())
}

var now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.FixedZone("AEDT", 11*3600))

func TestParse(t *testing.T) {
	im, err := impl.Parse("ref=intentions serve --mcp --workspace {workspace}")
	if err != nil || im.Label != "ref" || im.Command != "intentions serve --mcp --workspace {workspace}" {
		t.Fatalf("got %+v, %v", im, err)
	}
	for _, bad := range []string{"ref", "=x {workspace}", "ref=intentions serve --mcp"} {
		if _, err := impl.Parse(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestStartFakeServer(t *testing.T) {
	ws := testutil.Seed(t)
	srv, err := impl.Start(context.Background(), impl.Impl{Label: "fake", Command: fakeserver.Command("FAKE_VERSION=2.3.4")}, ws, now)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	if srv.Info != (impl.ServerInfo{Name: "fake-intentions", Version: "2.3.4"}) {
		t.Errorf("serverInfo %+v", srv.Info)
	}
	if !strings.Contains(srv.Instructions, srv.Workspace) || !strings.Contains(srv.Instructions, "Harness intentions-bench") {
		t.Errorf("{workspace} or INTENTIONS_HARNESS not passed: %q", srv.Instructions)
	}
	r, err := srv.CallTool(context.Background(), "intention_firm", json.RawMessage(`{"id":"`+testutil.TerminusID+`"}`))
	if err != nil || !r.IsError || r.Code != "refused" {
		t.Errorf("refusal: %+v %v", r, err)
	}
	if got := impl.Extensions(srv.Tools); !reflect.DeepEqual(got, []string{"mood_note"}) {
		t.Errorf("extensions %v", got)
	}
}

func TestStartFailureKeepsStderr(t *testing.T) {
	_, err := impl.Start(context.Background(), impl.Impl{Label: "broken", Command: fakeserver.Command("FAKE_FAIL=no-licence")}, testutil.Seed(t), now)
	var se *impl.StartError
	if !errors.As(err, &se) || !strings.Contains(se.Stderr, "no-licence") {
		t.Fatalf("want StartError with stderr, got %v", err)
	}
}

// assertVerbatim checks that every tool renders as a Messages API tool with
// the server's name, description, and input schema.
func assertVerbatim(t *testing.T, tools []*mcp.Tool) {
	t.Helper()
	for _, tool := range tools {
		def, err := impl.ToolDef(tool)
		if err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		want, err := impl.SchemaMap(tool)
		if err != nil {
			t.Fatal(err)
		}
		at, err := agent.AnthropicTool(def)
		if err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		var got struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"input_schema"`
		}
		b, _ := json.Marshal(at)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if got.Name != tool.Name || got.Description != tool.Description || !reflect.DeepEqual(got.InputSchema, want) {
			t.Errorf("%s not offered verbatim", tool.Name)
		}
	}
}

func TestToolsVerbatimFake(t *testing.T) {
	srv, err := impl.Start(context.Background(), impl.Impl{Label: "fake", Command: fakeserver.Command()}, testutil.Seed(t), now)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	assertVerbatim(t, srv.Tools)
}

// TestReferenceCLI runs against the reference implementation when
// `intentions` is on PATH: tools are offered verbatim, writes carry the
// harness and the pinned clock, and a harness firm is refused by code.
func TestReferenceCLI(t *testing.T) {
	if _, err := exec.LookPath("intentions"); err != nil {
		t.Skip("intentions not on PATH")
	}
	ws := t.TempDir()
	if err := run.CopyDir("../../scenarios/seeds/one-firm-terminus", ws); err != nil {
		t.Fatal(err)
	}
	srv, err := impl.Start(context.Background(), impl.Impl{Label: "ref", Command: "intentions serve --mcp --workspace {workspace}"}, ws, now)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	if srv.Info.Name == "" || srv.Instructions == "" {
		t.Errorf("serverInfo %+v, instructions %d bytes", srv.Info, len(srv.Instructions))
	}
	assertVerbatim(t, srv.Tools)
	ro := map[string]bool{}
	for _, tool := range srv.Tools {
		ro[tool.Name] = impl.ReadOnly(tool)
		if !impl.IsReferenceTool(tool.Name) {
			t.Logf("extension %s", tool.Name)
		}
	}
	if !ro["intention_list"] || ro["select"] || ro["intention_firm"] {
		t.Errorf("readOnlyHint: %v", ro)
	}
	ctx := context.Background()
	r, err := srv.CallTool(ctx, "desire_add", json.RawMessage(`{"title":"Call the accountant"}`))
	if err != nil || r.IsError {
		t.Fatalf("desire_add: %+v %v", r, err)
	}
	after, err := workspace.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range after.Objects {
		if o.Type != "desire" {
			continue
		}
		src, _ := o.Fields["source"].(map[string]any)
		if src["harness"] != impl.Harness {
			t.Errorf("desire source %v: not attributed to the harness", src)
		}
		if ts, _ := o.Timestamp(); !ts.Equal(now) {
			t.Errorf("timestamp %v, want %v", ts, now)
		}
	}
	terminus := ""
	for id, o := range after.Objects {
		if o.IsTerminus() {
			terminus = id
		}
	}
	r, err = srv.CallTool(ctx, "intention_add", json.RawMessage(`{"title":"Run","duration":"PT30M","window":{"calendar":"this-week"},"serves":[{"id":"`+terminus+`","role":"for-the-sake-of"}]}`))
	if err != nil || r.IsError {
		t.Fatalf("intention_add: %+v %v", r, err)
	}
	var added struct {
		ID string `json:"id"`
	}
	for _, line := range strings.Split(r.Text, "\n") {
		if json.Unmarshal([]byte(line), &added) == nil && added.ID != "" {
			break
		}
	}
	r, err = srv.CallTool(ctx, "intention_firm", json.RawMessage(`{"id":"`+added.ID+`"}`))
	if err != nil || !r.IsError || r.Code != "refused" {
		t.Errorf("harness firm: %+v %v", r, err)
	}
	// An agent passing a source with no harness is still the harness: the
	// restraint scores rest on this.
	r, err = srv.CallTool(ctx, "intention_firm", json.RawMessage(`{"id":"`+added.ID+`","source":{"author":"https://example.com/people/ada"}}`))
	if err != nil || !r.IsError || r.Code != "refused" {
		t.Errorf("firm with an author-only source was not refused: %+v %v", r, err)
	}
}
