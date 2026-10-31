package report_test

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/impl"
	"github.com/nodelogicau/intentions-bench/internal/report"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/score"
)

func TestWilson(t *testing.T) {
	a := report.Wilson(8, 10)
	if a.Rate != 0.8 || math.Abs(a.Low-0.4902) > 0.001 || math.Abs(a.High-0.9433) > 0.001 {
		t.Errorf("wilson %+v", a)
	}
	if z := report.Wilson(0, 0); z.N != 0 || z.Rate != 0 {
		t.Errorf("empty %+v", z)
	}
}

func TestOneOverreachInFive(t *testing.T) {
	s := []*scenario.Scenario{{ID: "s", Class: []string{"restraint/select"}}}
	rates := report.PassRates(map[string][]bool{"s": {true, true, true, false, true}}, s)
	r := rates["restraint/select"]
	if r.At1.Pass != 0.8 || r.At1.N != 1 || r.AllK.Pass != 0 || r.AllK.N != 1 {
		t.Errorf("rates %+v", r)
	}
	if rates[report.OverallRow] != r {
		t.Errorf("overall %+v", rates[report.OverallRow])
	}
}

func result(implLabel, id string, repeat int, errs score.Errors, failure string) score.Result {
	return score.Result{Impl: implLabel, Scenario: id, Repeat: repeat, Errors: errs, Failure: failure, FailureKind: map[bool]string{true: "clock"}[failure != ""],
		Cells:  []score.Cell{{Row: "desire", Col: "intention", Kind: score.Inflation}},
		Fields: []score.Field{{Field: "window", Relation: "narrower"}},
		Acts:   []score.Act{{Tool: "intention_firm", Outcome: score.Breach, Exchange: 2, Object: "int_1"}},
	}
}

func TestTwoImplementations(t *testing.T) {
	scenarios := []*scenario.Scenario{
		{ID: "a", Class: []string{"rung/desire"}},
		{ID: "b", Class: []string{"rung/desire", "restraint/select"}, Card: []scenario.Entry{{ID: "pick", Licenses: []scenario.Licence{{Act: "select", Key: "k"}}}}},
	}
	in := report.Input{
		Meta: run.Meta{Impls: []impl.Impl{{Label: "x", Command: "x {workspace}"}, {Label: "y", Command: "y {workspace}"}}, Repeats: 2,
			ScenarioSet: run.ScenarioSet{SHA256: "abc", Format: "intentions/0.2"}, Prompts: map[string]string{"phase": "123"}},
		Scenarios: scenarios,
		Classes:   map[string]string{"rung/desire": "Desires.", "restraint/select": "Select."},
		Records: map[string][]run.Record{
			"x": {{Impl: "x", Scenario: "a", Tools: []run.ToolInfo{{Name: "mood_note", Extension: true}}, Calls: []run.Call{{Tool: "mood_note"}}}},
		},
		Results: map[string][]score.Result{
			"x": {result("x", "a", 0, score.Errors{}, ""), result("x", "a", 1, score.Errors{Rungs: true}, ""),
				result("x", "b", 0, score.Errors{}, ""), result("x", "b", 1, score.Errors{}, "")},
			"y": {result("y", "a", 0, score.Errors{}, ""), result("y", "a", 1, score.Errors{}, "clock drift"),
				result("y", "b", 0, score.Errors{Restraint: true}, ""), result("y", "b", 1, score.Errors{}, "")},
		},
		Now: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
	}
	r := report.Build(in)
	if len(r.Impls) != 2 || r.Impls[0].Label != "x" || r.Impls[1].Label != "y" {
		t.Fatalf("impls %+v", r.Impls)
	}
	x, y := r.Impls[0], r.Impls[1]
	if got := x.Pass["rungs"]["rung/desire"]; got.AllK.Pass != 1 || got.AllK.N != 2 || got.At1.Pass != 1.5 {
		t.Errorf("x rungs %+v", got)
	}
	if got := y.Pass["restraint"]["restraint/select"]; got.AllK.Pass != 0 || got.At1.Pass != 0.5 {
		t.Errorf("y restraint %+v", got)
	}
	if y.Scored != 3 || len(y.Failures) != 1 || y.Pass["rungs"][report.OverallRow].At1.N != 2 {
		t.Errorf("failures not excluded: scored %d failures %v", y.Scored, y.Failures)
	}
	if x.Matrix["desire"]["intention"] != 4 || x.Inflation != 4 || x.Windows["narrower"] != 4 || len(x.Breaches) != 4 || x.Extensions["mood_note"] != 1 {
		t.Errorf("x aggregates %+v", x)
	}
	md := report.Markdown(r)
	for _, want := range []string{"| Class | x | y |", "### Restraint", "## Rung matrix", "### x", "### y", "Inflation 4", "`mood_note` 1", "b: `pick`", "Conformance breaches", "Selection is licensed by key"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	b, err := json.Marshal(r)
	if err != nil || !strings.Contains(string(b), `"schema":"intentions-bench-report/1"`) {
		t.Errorf("json: %v", err)
	}
}
