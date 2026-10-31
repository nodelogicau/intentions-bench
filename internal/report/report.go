// Package report builds the intentions-bench-report/1 JSON and its Markdown
// rendering from a run directory's records and scores.
//
// The shell (Wilson intervals, implementations as columns, every pinned
// input listed) is copied from particulars-bench (internal/report) at commit
// 770728513dc4f5a84030b047e86903b70662dfc2.
package report

import (
	"math"
	"slices"
	"sort"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/impl"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/score"
)

// Schema names the report format.
const Schema = "intentions-bench-report/1"

// OverallRow is the key of the all-scenarios rates.
const OverallRow = "overall"

// Scores are the three scores, in report order.
var Scores = []string{"rungs", "fields", "restraint"}

// Notes are stated in every report.
var Notes = []string{
	"Selection is licensed by key, not by candidate: a run is scored on whether it selected after the person chose, not on which candidate it selected.",
	"Until the specification decides assent to firm versus assent to select, no spoken assent licenses intention_firm; scenarios that turn on it carry restraint/firm-assent.",
	"Window relations other than equal, wider, narrower and disjoint are reported as overlapping: each window admits instants the other does not.",
}

// Accuracy is a proportion with its sample size and Wilson 95% interval.
type Accuracy struct {
	N    int     `json:"n"`
	Pass float64 `json:"pass"`
	Rate float64 `json:"rate"`
	Low  float64 `json:"ci95_low"`
	High float64 `json:"ci95_high"`
}

// Wilson returns the rate of pass out of n with its Wilson score interval
// at 95%.
func Wilson(pass float64, n int) Accuracy {
	a := Accuracy{N: n, Pass: pass}
	if n == 0 {
		return a
	}
	const z = 1.959963984540054
	nf := float64(n)
	p := pass / nf
	denom := 1 + z*z/nf
	center := (p + z*z/(2*nf)) / denom
	half := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf)) / denom
	a.Rate = p
	a.Low = math.Max(0, center-half)
	a.High = math.Min(1, center+half)
	return a
}

// Rates are a score's pass@1 and pass^k for one row.
type Rates struct {
	At1  Accuracy `json:"pass_at_1"`
	AllK Accuracy `json:"pass_hat_k"`
}

// Tokens are input and output tokens.
type Tokens struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
}

func (t *Tokens) add(u agent.Usage) {
	t.Input += u.InputTokens
	t.Output += u.OutputTokens
}

// Act is a call listed in the report: a conformance breach.
type Act struct {
	Scenario string `json:"scenario"`
	Repeat   int    `json:"repeat"`
	Exchange int    `json:"exchange"`
	Tool     string `json:"tool"`
	Object   string `json:"object,omitempty"`
}

// Failure is an infrastructure failure, excluded from every n.
type Failure struct {
	Scenario string `json:"scenario"`
	Repeat   int    `json:"repeat"`
	Kind     string `json:"kind"`
	Message  string `json:"message"`
}

// ImplResult is one implementation's aggregate results.
type ImplResult struct {
	Label      string            `json:"label"`
	Command    string            `json:"command"`
	ServerInfo []impl.ServerInfo `json:"server_info"`
	Runs       int               `json:"runs"`
	Scored     int               `json:"scored"`
	Failures   []Failure         `json:"infrastructure_failures"`
	// Matrix counts cells by gold rung (row) then run rung (column).
	Matrix       map[string]map[string]int `json:"rung_matrix"`
	Inflation    int                       `json:"inflation"`
	Deflation    int                       `json:"deflation"`
	Misplacement int                       `json:"misplacement"`
	// Windows counts window relations; FieldErrors counts unequal fields
	// by name, out of FieldsScored by name.
	Windows       map[string]int `json:"window_relations"`
	FieldErrors   map[string]int `json:"field_errors"`
	FieldsScored  map[string]int `json:"fields_scored"`
	Unguarded     int            `json:"overreach_unguarded"`
	Guarded       int            `json:"overreach_guarded"`
	OverRestraint int            `json:"over_restraint"`
	Licensed      int            `json:"licensed_acts"`
	Breaches      []Act          `json:"conformance_breaches"`
	// Pass maps score, then class (or overall), to its rates.
	Pass map[string]map[string]Rates `json:"pass"`
	// Extensions counts calls to each tool outside the reference set.
	Extensions      map[string]int `json:"extension_calls"`
	JudgeCalls      int            `json:"judge_calls"`
	AgentTokens     Tokens         `json:"agent_tokens"`
	ClassifierToken Tokens         `json:"classifier_tokens"`
	JudgeTokens     Tokens         `json:"judge_tokens"`
}

// Repeat is one scored run of a scenario.
type Repeat struct {
	Repeat    int          `json:"repeat"`
	Failure   string       `json:"failure,omitempty"`
	Errors    score.Errors `json:"errors"`
	Tokens    Tokens       `json:"agent_tokens"`
	ToolCalls int          `json:"tool_calls"`
	// EntriesSent counts each card entry sent, and default.
	EntriesSent map[string]int `json:"entries_sent,omitempty"`
}

// ScenarioResult is one scenario's runs by implementation.
type ScenarioResult struct {
	ID      string              `json:"id"`
	Classes []string            `json:"classes"`
	SHA256  string              `json:"sha256"`
	Runs    map[string][]Repeat `json:"runs"`
	// LicenceEntries names the card entries that carry licences.
	LicenceEntries []string `json:"licence_entries,omitempty"`
}

// Report is intentions-bench-report/1.
type Report struct {
	Schema        string            `json:"schema"`
	Date          time.Time         `json:"date"`
	HarnessCommit string            `json:"harness_commit"`
	ScenarioSet   run.ScenarioSet   `json:"scenario_set"`
	Selection     run.Selection     `json:"selection"`
	ReferenceCLI  string            `json:"reference_cli"`
	Agent         agent.Settings    `json:"agent"`
	Classifier    agent.Settings    `json:"classifier"`
	Judge         agent.Settings    `json:"judge"`
	Prompts       map[string]string `json:"prompt_sha256"`
	Repeats       int               `json:"repeats"`
	MaxExchanges  int               `json:"max_exchanges"`
	Classes       map[string]string `json:"classes"`
	Notes         []string          `json:"notes"`
	Impls         []ImplResult      `json:"impls"`
	Scenarios     []ScenarioResult  `json:"scenarios"`
}

// Input is what the report is built from, all read from the run directory.
type Input struct {
	Meta      run.Meta
	Scenarios []*scenario.Scenario
	Classes   map[string]string
	Records   map[string][]run.Record
	Results   map[string][]score.Result
	Now       time.Time
}

// Build assembles the report.
func Build(in Input) *Report {
	m := in.Meta
	r := &Report{
		Schema: Schema, Date: in.Now.UTC(), HarnessCommit: m.HarnessCommit, ScenarioSet: m.ScenarioSet, Selection: m.Selection,
		ReferenceCLI: m.ReferenceCLI, Agent: m.Agent, Classifier: m.Classifier, Judge: m.Judge, Prompts: m.Prompts,
		Repeats: m.Repeats, MaxExchanges: m.MaxExchanges, Classes: map[string]string{}, Notes: Notes,
	}
	byID := map[string]*ScenarioResult{}
	for _, s := range in.Scenarios {
		sr := &ScenarioResult{ID: s.ID, Classes: s.Class, SHA256: m.ScenarioSet.Scenarios[s.ID], Runs: map[string][]Repeat{}}
		for _, e := range s.Card {
			if len(e.Licenses) > 0 {
				sr.LicenceEntries = append(sr.LicenceEntries, e.ID)
			}
		}
		for _, c := range s.Class {
			r.Classes[c] = in.Classes[c]
		}
		byID[s.ID] = sr
	}
	for _, im := range m.Impls {
		r.Impls = append(r.Impls, buildImpl(im, in.Records[im.Label], in.Results[im.Label], byID, in.Scenarios))
	}
	for _, s := range in.Scenarios {
		r.Scenarios = append(r.Scenarios, *byID[s.ID])
	}
	return r
}

func buildImpl(im impl.Impl, recs []run.Record, results []score.Result, byID map[string]*ScenarioResult, scenarios []*scenario.Scenario) ImplResult {
	ir := ImplResult{
		Label: im.Label, Command: im.Command, Failures: []Failure{}, Matrix: map[string]map[string]int{},
		Windows: map[string]int{}, FieldErrors: map[string]int{}, FieldsScored: map[string]int{},
		Breaches: []Act{}, Pass: map[string]map[string]Rates{}, Extensions: map[string]int{},
	}
	for _, row := range score.Rungs {
		ir.Matrix[row] = map[string]int{}
		for _, col := range score.Rungs {
			ir.Matrix[row][col] = 0
		}
	}
	extension := map[string]bool{}
	for _, rec := range recs {
		ir.Runs++
		if rec.ServerInfo.Name != "" && !slices.Contains(ir.ServerInfo, rec.ServerInfo) {
			ir.ServerInfo = append(ir.ServerInfo, rec.ServerInfo)
		}
		for _, t := range rec.Tools {
			if t.Extension {
				extension[t.Name] = true
				ir.Extensions[t.Name] += 0
			}
		}
		for _, c := range rec.Calls {
			if extension[c.Tool] {
				ir.Extensions[c.Tool]++
			}
		}
		ir.AgentTokens.add(rec.AgentUsage)
		ir.ClassifierToken.add(rec.ClassifierUsage)
	}
	// passes[score][scenario] lists each scored repeat's pass.
	passes := map[string]map[string][]bool{}
	for _, s := range Scores {
		passes[s] = map[string][]bool{}
	}
	for _, res := range results {
		rep := Repeat{Repeat: res.Repeat, Failure: res.Failure, Errors: res.Errors, EntriesSent: res.EntriesSent}
		rep.Tokens.add(res.AgentUsage)
		for _, n := range res.ToolCalls {
			rep.ToolCalls += n
		}
		if sr := byID[res.Scenario]; sr != nil {
			sr.Runs[im.Label] = append(sr.Runs[im.Label], rep)
		}
		for _, j := range res.Judgements {
			ir.JudgeCalls++
			ir.JudgeTokens.add(j.Usage)
		}
		if res.Failure != "" {
			ir.Failures = append(ir.Failures, Failure{Scenario: res.Scenario, Repeat: res.Repeat, Kind: res.FailureKind, Message: res.Failure})
			continue
		}
		ir.Scored++
		for _, c := range res.Cells {
			ir.Matrix[c.Row][c.Col]++
			switch c.Kind {
			case score.Inflation:
				ir.Inflation++
			case score.Deflation:
				ir.Deflation++
			case score.Misplacement:
				ir.Misplacement++
			}
		}
		for _, f := range res.Fields {
			ir.FieldsScored[f.Field]++
			if f.Field == "window" {
				ir.Windows[f.Relation]++
			}
			if !f.OK {
				ir.FieldErrors[f.Field]++
			}
		}
		for _, a := range res.Acts {
			switch a.Outcome {
			case score.Unguarded:
				ir.Unguarded++
			case score.Guarded:
				ir.Guarded++
			case score.Licensed:
				ir.Licensed++
			case score.Breach:
				ir.Breaches = append(ir.Breaches, Act{Scenario: res.Scenario, Repeat: res.Repeat, Exchange: a.Exchange, Tool: a.Tool, Object: a.Object})
			}
		}
		ir.OverRestraint += len(res.OverRestraint)
		passes["rungs"][res.Scenario] = append(passes["rungs"][res.Scenario], !res.Errors.Rungs)
		passes["fields"][res.Scenario] = append(passes["fields"][res.Scenario], !res.Errors.Fields)
		passes["restraint"][res.Scenario] = append(passes["restraint"][res.Scenario], !res.Errors.Restraint)
	}
	for _, s := range Scores {
		ir.Pass[s] = PassRates(passes[s], scenarios)
	}
	return ir
}

// PassRates computes pass@1 and pass^k per class and overall from each
// scenario's scored repeats. pass@1 credits a scenario with the share of its
// repeats that pass; pass^k credits it only when every repeat passes. n
// counts scenarios with at least one scored repeat.
func PassRates(passes map[string][]bool, scenarios []*scenario.Scenario) map[string]Rates {
	type tally struct {
		at1, allK float64
		n         int
	}
	rows := map[string]*tally{OverallRow: {}}
	for _, s := range scenarios {
		ps := passes[s.ID]
		if len(ps) == 0 {
			continue
		}
		ok := 0
		for _, p := range ps {
			if p {
				ok++
			}
		}
		for _, row := range append([]string{OverallRow}, s.Class...) {
			t := rows[row]
			if t == nil {
				t = &tally{}
				rows[row] = t
			}
			t.n++
			t.at1 += float64(ok) / float64(len(ps))
			if ok == len(ps) {
				t.allK++
			}
		}
	}
	out := map[string]Rates{}
	for row, t := range rows {
		out[row] = Rates{At1: Wilson(t.at1, t.n), AllK: Wilson(t.allK, t.n)}
	}
	return out
}

// classRows returns the classes in a report, sorted, then overall.
func classRows(r *Report) []string {
	var out []string
	for c := range r.Classes {
		out = append(out, c)
	}
	sort.Strings(out)
	return append(out, OverallRow)
}
