package cli_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/cli"
	"github.com/nodelogicau/intentions-bench/internal/fakeserver"
	"github.com/nodelogicau/intentions-bench/internal/report"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/score"
	"github.com/nodelogicau/intentions-bench/internal/testutil"
)

func TestMain(m *testing.M) {
	fakeserver.Main()
	os.Exit(m.Run())
}

type env struct {
	out, errb bytes.Buffer
	vars      map[string]string
	providers int
	model     *testutil.Model
}

func (e *env) deps() cli.Deps {
	return cli.Deps{
		Stdout: &e.out, Stderr: &e.errb,
		Getenv: func(k string) string { return e.vars[k] },
		NewProvider: func(agent.Settings, string) (agent.Provider, error) {
			e.providers++
			return e.model, nil
		},
		Now: func() time.Time { return time.Date(2026, 10, 31, 1, 2, 3, 0, time.UTC) },
	}
}

func TestDryRunWithoutKey(t *testing.T) {
	e := &env{vars: map[string]string{}}
	code := cli.Run(context.Background(), []string{"run", "--dry-run", "--scenarios", "../../scenarios", "--results", t.TempDir(),
		"--impl", "ref=intentions serve --mcp --workspace {workspace}", "--classes", "rung/none"}, e.deps())
	if code != 0 {
		t.Fatalf("exit %d: %s", code, e.errb.String())
	}
	if !strings.Contains(e.out.String(), "Plan (run)") || !strings.Contains(e.out.String(), "3 scenarios") || e.providers != 0 {
		t.Errorf("plan %q, providers %d", e.out.String(), e.providers)
	}
}

func TestRefusesWithoutKey(t *testing.T) {
	e := &env{vars: map[string]string{}}
	code := cli.Run(context.Background(), []string{"run", "--scenarios", "../../scenarios", "--impl", "ref=x {workspace}"}, e.deps())
	if code != 1 || !strings.Contains(e.errb.String(), "ANTHROPIC_API_KEY is not set") || e.providers != 0 {
		t.Errorf("exit %d, stderr %q", code, e.errb.String())
	}
}

func TestFlags(t *testing.T) {
	e := &env{vars: map[string]string{}}
	for _, args := range [][]string{
		{"run", "--impl", "a=x"},
		{"run", "--impl", "a=x {workspace}", "--impl", "a=y {workspace}"},
		{"run", "--effort", "extreme"},
		{"run", "--repeats", "0"},
		{"walk"},
	} {
		if code := cli.Run(context.Background(), args, e.deps()); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

// copySet copies the shipped scenario set so the test can edit it.
func copySet(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "scenarios")
	if err := run.CopyDir("../../scenarios", dst); err != nil {
		t.Fatal(err)
	}
	return dst
}

// TestPipeline runs, scores and reports one scenario against the fake
// server with a scripted model that plays agent and classifier.
func TestPipeline(t *testing.T) {
	if _, err := exec.LookPath("intentions"); err != nil {
		t.Skip("intentions not on PATH: the run phase checks the set with it")
	}
	set := copySet(t)
	results := t.TempDir()
	model := &testutil.Model{Script: func(p anthropic.MessageNewParams) testutil.Reply {
		text, toolResults := testutil.LastUser(p)
		switch {
		case strings.Contains(text, "<message>"):
			return testutil.Reply{Text: "none"}
		case toolResults:
			return testutil.Reply{Text: "I've noted that you want to call the accountant."}
		default:
			return testutil.Reply{Tools: []testutil.ToolUse{{Name: "desire_add", Input: map[string]any{"title": "Call the accountant"}}}}
		}
	}}
	e := &env{vars: map[string]string{"ANTHROPIC_API_KEY": "test"}, model: model}
	runDir := filepath.Join(results, "r1")
	args := []string{"--scenarios", set, "--results", results, "--run", runDir}
	code := cli.Run(context.Background(), append([]string{"run", "--impl", "fake=" + fakeserver.Command(), "--ids", "desire-accountant-01", "--repeats", "2"}, args...), e.deps())
	if code != 0 {
		t.Fatalf("run exit %d: %s", code, e.errb.String())
	}
	dir := &run.Dir{Path: runDir}
	meta, _, _ := dir.Meta()
	recs, err := dir.Records("fake")
	if err != nil || len(recs) != 2 || recs[0].Failure != "" {
		t.Fatalf("records %+v %v", recs, err)
	}
	if meta.ScenarioSet.SHA256 == "" || meta.Prompts["classifier"] == "" || meta.ReferenceCLI == "" {
		t.Errorf("meta %+v", meta)
	}

	// Edit the source scenario's gold after the run: score must use the
	// copy fixed in the run directory.
	if err := os.WriteFile(filepath.Join(set, "gold", "desire-accountant-01.yaml"), []byte("[]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cli.Run(context.Background(), append([]string{"score"}, args...), e.deps()); code != 0 {
		t.Fatalf("score exit %d: %s", code, e.errb.String())
	}
	results2, err := run.Read[score.Result](dir, "scores/fake")
	if err != nil || len(results2) != 2 {
		t.Fatalf("scores %v %v", results2, err)
	}
	for _, r := range results2 {
		if len(r.Cells) != 1 || r.Cells[0].Kind != score.Correct || r.Errors.Rungs {
			t.Errorf("scored against the edited gold: %+v", r.Cells)
		}
	}
	e.out.Reset()
	if code := cli.Run(context.Background(), append([]string{"report"}, args...), e.deps()); code != 0 {
		t.Fatalf("report exit %d: %s", code, e.errb.String())
	}
	var r report.Report
	if _, err := run.ReadJSON(filepath.Join(runDir, "report.json"), &r); err != nil {
		t.Fatal(err)
	}
	if r.Schema != report.Schema || r.ScenarioSet.SHA256 != meta.ScenarioSet.SHA256 {
		t.Errorf("report schema %s, set hash %s (want the original %s)", r.Schema, r.ScenarioSet.SHA256, meta.ScenarioSet.SHA256)
	}
	if got := r.Impls[0].Pass["rungs"]["rung/desire"]; got.AllK.Rate != 1 || got.AllK.N != 1 {
		t.Errorf("pass^2 %+v", got)
	}
	if !strings.Contains(e.out.String(), "| Class | fake |") {
		t.Errorf("markdown:\n%s", e.out.String())
	}
}
