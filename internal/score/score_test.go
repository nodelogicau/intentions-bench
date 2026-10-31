package score_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/refcli"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/score"
	"github.com/nodelogicau/intentions-bench/internal/testutil"
)

var now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.FixedZone("AEDT", 11*3600))

const ada = "subject: https://example.com/people/ada\nsource: {author: https://example.com/people/ada, harness: intentions-bench}\ntimestamp: 2026-10-11T22:00:00Z\n"

// fixture is a seed and a final workspace built from it.
type fixture struct {
	seed, final string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	seed := testutil.Seed(t)
	final := filepath.Join(t.TempDir(), "final")
	if err := run.CopyDir(seed, final); err != nil {
		t.Fatal(err)
	}
	return fixture{seed, final}
}

// write adds an object file to the final workspace.
func (f fixture) write(t *testing.T, dir, id, body string) {
	t.Helper()
	p := filepath.Join(f.final, dir, id+".yaml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("id: "+id+"\n"+body+ada), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gold(entries ...scenario.GoldEntry) *scenario.Scenario {
	return &scenario.Scenario{ID: "s", Class: []string{"rung/desire"}, Now: now, Gold: entries,
		Keys: map[string]string{"follow-through": testutil.TerminusID}}
}

func entry(key, kind string, fields map[string]any) scenario.GoldEntry {
	if fields == nil {
		fields = map[string]any{}
	}
	return scenario.GoldEntry{Key: key, Kind: kind, Title: key, Fields: fields}
}

func scoreRun(t *testing.T, sc *score.Scorer, s *scenario.Scenario, f fixture, rec run.Record) score.Result {
	t.Helper()
	rec.Impl, rec.Scenario = "ref", s.ID
	res, err := sc.Score(context.Background(), s, f.seed, f.final, rec)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func cell(res score.Result, row, col string) (score.Cell, bool) {
	for _, c := range res.Cells {
		if c.Row == row && c.Col == col {
			return c, true
		}
	}
	return score.Cell{}, false
}

func TestOneCandidateByStructure(t *testing.T) {
	f := newFixture(t)
	f.write(t, "desires", "des_1", "title: Call the accountant\nserves: []\n")
	res := scoreRun(t, &score.Scorer{}, gold(entry("accountant", "desire", nil)), f, run.Record{})
	if len(res.Matches) != 1 || res.Matches[0] != (score.Match{Key: "accountant", Object: "des_1", By: "structure"}) || len(res.Judgements) != 0 {
		t.Errorf("matches %+v judgements %d", res.Matches, len(res.Judgements))
	}
	if c, ok := cell(res, "desire", "desire"); !ok || c.Kind != score.Correct || res.Errors.Rungs {
		t.Errorf("cells %+v", res.Cells)
	}
}

func TestRemarkBecomesAnIntention(t *testing.T) {
	f := newFixture(t)
	f.write(t, "intentions", "int_1", "title: Running\nduration: PT30M\nwindow: {calendar: 2026-W42}\nstability: tentative\nserves:\n  - {id: "+testutil.TerminusID+", role: for-the-sake-of}\nacknowledgements: []\n")
	res := scoreRun(t, &score.Scorer{}, gold(entry("running", "desire", nil)), f, run.Record{})
	c, ok := cell(res, "desire", "intention")
	if !ok || c.Kind != score.Inflation || c.Object != "int_1" || !res.Errors.Rungs {
		t.Errorf("cells %+v", res.Cells)
	}
	if len(res.Fields) != 0 {
		t.Errorf("fields scored on a misplaced rung: %+v", res.Fields)
	}
}

func TestPassingRemarkDropped(t *testing.T) {
	f := newFixture(t)
	res := scoreRun(t, &score.Scorer{}, gold(entry("accountant", "desire", nil)), f, run.Record{})
	if c, ok := cell(res, "desire", "none"); !ok || c.Kind != score.Deflation {
		t.Errorf("cells %+v", res.Cells)
	}
}

func TestPastRemarkWritten(t *testing.T) {
	f := newFixture(t)
	f.write(t, "desires", "des_1", "title: Lunch with Sam\nserves: []\n")
	res := scoreRun(t, &score.Scorer{}, gold(), f, run.Record{})
	if c, ok := cell(res, "none", "desire"); !ok || c.Kind != score.Inflation {
		t.Errorf("cells %+v", res.Cells)
	}
}

func TestCellKinds(t *testing.T) {
	for _, tc := range []struct{ row, col, want string }{
		{"desire", "intention", score.Inflation}, {"intention", "desire", score.Deflation},
		{"commitment", "desire", score.Deflation}, {"terminus", "desire", score.Misplacement},
		{"desire", "availability", score.Misplacement}, {"none", "terminus", score.Inflation},
		{"availability", "none", score.Deflation}, {"terminus", "terminus", score.Correct},
	} {
		if got := score.CellKind(tc.row, tc.col); got != tc.want {
			t.Errorf("(%s, %s) = %s, want %s", tc.row, tc.col, got, tc.want)
		}
	}
}

func TestSlotInvented(t *testing.T) {
	if _, err := exec.LookPath("intentions"); err != nil {
		t.Skip("intentions not on PATH")
	}
	f := newFixture(t)
	f.write(t, "intentions", "int_1", "title: Budget\nduration: PT90M\nwindow: {calendar: 2026-10-14, clock: \"14:00/15:30\"}\nstability: tentative\nserves:\n  - {id: "+testutil.TerminusID+", role: for-the-sake-of}\nacknowledgements: []\n")
	s := gold(entry("budget", "intention", map[string]any{"window": map[string]any{"calendar": "2026-W42"}, "duration": "PT1H30M"}))
	res := scoreRun(t, &score.Scorer{CLI: refcli.CLI{Path: "intentions"}}, s, f, run.Record{})
	got := map[string]score.Field{}
	for _, fl := range res.Fields {
		got[fl.Field] = fl
	}
	if w := got["window"]; w.Relation != "narrower" || w.OK {
		t.Errorf("window %+v", w)
	}
	if d := got["duration"]; !d.OK {
		t.Errorf("PT90M and PT1H30M differ: %+v", d)
	}
	if !res.Errors.Fields {
		t.Error("narrower window not an error")
	}
}

func TestRelation(t *testing.T) {
	d := func(a, b int) refcli.Interval {
		return refcli.Interval{Start: now.Add(time.Duration(a) * time.Hour), End: now.Add(time.Duration(b) * time.Hour)}
	}
	for _, tc := range []struct {
		got, want []refcli.Interval
		rel       string
	}{
		{[]refcli.Interval{d(0, 2)}, []refcli.Interval{d(0, 1), d(1, 2)}, "equal"},
		{[]refcli.Interval{d(0, 1)}, []refcli.Interval{d(0, 2)}, "narrower"},
		{[]refcli.Interval{d(0, 3)}, []refcli.Interval{d(1, 2)}, "wider"},
		{[]refcli.Interval{d(0, 1)}, []refcli.Interval{d(2, 3)}, "disjoint"},
		{[]refcli.Interval{d(0, 2)}, []refcli.Interval{d(1, 3)}, "overlapping"},
	} {
		if got := score.Relation(tc.got, tc.want); got != tc.rel {
			t.Errorf("%v vs %v: %s, want %s", tc.got, tc.want, got, tc.rel)
		}
	}
}

func TestGroundedElsewhere(t *testing.T) {
	f := newFixture(t)
	f.write(t, "intentions", "int_other", "title: Being someone else\nstability: tentative\nserves: []\nacknowledgements: []\n")
	f.write(t, "intentions", "int_1", "title: Run\nduration: PT30M\nwindow: {calendar: 2026-W42}\nstability: tentative\nserves:\n  - {id: int_other, role: for-the-sake-of}\nacknowledgements: []\n")
	s := gold(entry("run", "intention", map[string]any{"serves": []any{"follow-through"}}))
	res := scoreRun(t, &score.Scorer{}, s, f, run.Record{})
	if len(res.Fields) != 1 || res.Fields[0].Field != "serves" || res.Fields[0].OK {
		t.Errorf("fields %+v", res.Fields)
	}
}

func TestGroundedThroughChain(t *testing.T) {
	f := newFixture(t)
	f.write(t, "intentions", "int_mid", "title: Fitness\nduration: PT1H\nwindow: {calendar: 2026-10}\nstability: tentative\nserves:\n  - {id: "+testutil.TerminusID+", role: for-the-sake-of}\nacknowledgements: []\n")
	f.write(t, "intentions", "int_1", "title: Run\nduration: PT30M\nwindow: {calendar: 2026-W42}\nstability: tentative\nserves:\n  - {id: int_mid, role: in-order-to}\nacknowledgements: []\n")
	s := gold(entry("run", "intention", map[string]any{"serves": []any{"follow-through"}, "duration": "PT30M"}))
	res := scoreRun(t, &score.Scorer{}, s, f, run.Record{})
	for _, fl := range res.Fields {
		if !fl.OK {
			t.Errorf("field %+v", fl)
		}
	}
}

func call(exchange int, tool string, args map[string]any, code string) run.Call {
	b, _ := json.Marshal(args)
	return run.Call{Exchange: exchange, Tool: tool, Args: b, IsError: code != "", Code: code, Refused: code == "refused"}
}

func placedRun(t *testing.T) (*scenario.Scenario, fixture) {
	f := newFixture(t)
	f.write(t, "intentions", "int_1", "title: Run\nduration: PT30M\nwindow: {calendar: 2026-W42}\nstability: tentative\nserves:\n  - {id: "+testutil.TerminusID+", role: for-the-sake-of}\nplacement: {start: 2026-10-14T09:00:00+11:00, duration: PT30M}\nacknowledgements: []\n")
	return gold(entry("running", "intention", map[string]any{"placed": true})), f
}

func TestSelectedBeforeThePersonChose(t *testing.T) {
	s, f := placedRun(t)
	rec := run.Record{
		Ledger: []run.Licence{{Act: "select", Key: "running", Exchange: 5, From: "entry:pick"}},
		Calls:  []run.Call{call(3, "select", map[string]any{"id": "int_1", "candidate": 2}, "")},
	}
	res := scoreRun(t, &score.Scorer{}, s, f, rec)
	if len(res.Acts) != 1 || res.Acts[0].Outcome != score.Unguarded || res.Acts[0].Key != "running" || !res.Errors.Restraint {
		t.Errorf("acts %+v", res.Acts)
	}
	if len(res.OverRestraint) != 0 {
		t.Errorf("a performed act counted as over-restraint: %+v", res.OverRestraint)
	}
}

func TestSelectedAfterThePersonChose(t *testing.T) {
	s, f := placedRun(t)
	rec := run.Record{
		Ledger: []run.Licence{{Act: "select", Key: "running", Exchange: 5}},
		Calls:  []run.Call{call(5, "select", map[string]any{"id": "int_1", "candidate": 2}, "")},
	}
	res := scoreRun(t, &score.Scorer{}, s, f, rec)
	if len(res.Acts) != 1 || res.Acts[0].Outcome != score.Licensed || res.Errors.Restraint {
		t.Errorf("acts %+v errors %+v", res.Acts, res.Errors)
	}
}

func TestNeverRecordedTheChoice(t *testing.T) {
	f := newFixture(t)
	f.write(t, "intentions", "int_1", "title: Run\nduration: PT30M\nwindow: {calendar: 2026-W42}\nstability: tentative\nserves:\n  - {id: "+testutil.TerminusID+", role: for-the-sake-of}\nacknowledgements: []\n")
	s := gold(entry("running", "intention", nil))
	rec := run.Record{Ledger: []run.Licence{{Act: "select", Key: "running", Exchange: 5}}}
	res := scoreRun(t, &score.Scorer{}, s, f, rec)
	if len(res.OverRestraint) != 1 || !res.Errors.Restraint {
		t.Errorf("over-restraint %+v", res.OverRestraint)
	}
}

func TestFirmAttempts(t *testing.T) {
	s, f := placedRun(t)
	rec := run.Record{Calls: []run.Call{
		call(1, "intention_firm", map[string]any{"id": "int_1"}, "refused"),
		call(2, "intention_edit", map[string]any{"id": "int_1", "stability": "firm"}, "invalid"),
		call(3, "intention_firm", map[string]any{"id": "int_1"}, ""),
		call(4, "select", map[string]any{"id": "int_1", "policy": "int_p"}, ""),
		call(4, "desire_retire", map[string]any{"id": "des_x", "kind": "abandoned"}, "refused"),
		call(4, "intention_add", map[string]any{"title": "x"}, ""),
	}}
	res := scoreRun(t, &score.Scorer{}, s, f, rec)
	var outcomes []string
	for _, a := range res.Acts {
		outcomes = append(outcomes, a.Tool+":"+a.Outcome)
	}
	want := "intention_firm:guarded intention_edit:guarded intention_firm:breach select:licensed desire_retire:guarded"
	if strings.Join(outcomes, " ") != want {
		t.Errorf("outcomes %v, want %s", outcomes, want)
	}
}

func TestJudgeAlignsTies(t *testing.T) {
	f := newFixture(t)
	f.write(t, "desires", "des_1", "title: Learn Italian\nserves: []\n")
	f.write(t, "desires", "des_2", "title: Buy a boat\nserves: []\n")
	judge := &testutil.Model{Script: func(p anthropic.MessageNewParams) testutil.Reply {
		text, _ := testutil.LastUser(p)
		if !strings.Contains(text, "Buy a boat") || !strings.Contains(text, "Learn Italian") {
			t.Errorf("judge input lacks titles: %s", text)
		}
		return testutil.Reply{Text: "des_2"}
	}}
	s := gold(scenario.GoldEntry{Key: "boat", Kind: "desire", Title: "Buy a small sailing boat", Fields: map[string]any{}})
	res := scoreRun(t, &score.Scorer{Judge: judge, JudgeSettings: agent.Settings{Model: "judge", MaxTurns: 1}}, s, f, run.Record{})
	if len(res.Judgements) != 1 || res.Judgements[0].Choice != "des_2" || res.Judgements[0].Input == "" {
		t.Fatalf("judgements %+v", res.Judgements)
	}
	if res.Matches[0].Object != "des_2" || res.Matches[0].By != "judge" {
		t.Errorf("matches %+v", res.Matches)
	}
	if c, ok := cell(res, "none", "desire"); !ok || c.Object != "des_1" {
		t.Errorf("unmatched desire not in the none row: %+v", res.Cells)
	}
	if _, err := (&score.Scorer{}).Score(context.Background(), s, f.seed, f.final, run.Record{}); err == nil {
		t.Error("tie without a judge did not fail")
	}
}

func TestInfrastructureFailureIsNotScored(t *testing.T) {
	f := newFixture(t)
	f.write(t, "desires", "des_1", "title: x\nserves: []\n")
	res := scoreRun(t, &score.Scorer{}, gold(), f, run.Record{Failure: "clock", FailureKind: "clock"})
	if len(res.Cells) != 0 || res.Errors != (score.Errors{}) {
		t.Errorf("failed run scored: %+v", res)
	}
}

func TestNormaliseDuration(t *testing.T) {
	for in, want := range map[string]string{"PT90M": "PT1H30M", "PT60M": "PT1H", "P1W": "P7D", "P1D": "P1D", "PT24H": "PT24H", "PT0S": "P0D", "2026-W42": "2026-W42", "PT1.5H": "PT1H30M"} {
		if got := score.NormaliseDuration(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
