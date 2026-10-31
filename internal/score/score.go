// Package score scores one scenario run from its run directory: it matches
// the objects the run wrote to gold keys, places them in the rung matrix,
// compares the fields of correctly placed objects, and classifies every
// mutating call for restraint. The judge only aligns objects to keys when
// structure ties; every score is computed from structure.
package score

import (
	"context"
	"slices"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/refcli"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/workspace"
)

// Rungs are the rows and columns of the rung matrix, in order.
var Rungs = []string{"none", "desire", "terminus", "intention", "availability", "commitment"}

// ladder is the commissive ladder; terminus and availability are off it.
var ladder = map[string]int{"none": 0, "desire": 1, "intention": 2, "commitment": 3}

// Cell kinds.
const (
	Correct      = "correct"
	Inflation    = "inflation"
	Deflation    = "deflation"
	Misplacement = "misplacement"
)

// CellKind classifies a rung matrix cell by its row (gold) and column (run).
func CellKind(row, col string) string {
	switch {
	case row == col:
		return Correct
	case row == "none":
		return Inflation
	case col == "none":
		return Deflation
	}
	r, rok := ladder[row]
	c, cok := ladder[col]
	if !rok || !cok {
		return Misplacement
	}
	if c > r {
		return Inflation
	}
	return Deflation
}

// Match is a gold key and the object matched to it, if any.
type Match struct {
	Key    string `json:"key"`
	Object string `json:"object,omitempty"`
	// By is "seed" (a seed key), "structure", or "judge".
	By string `json:"by,omitempty"`
}

// Cell is one entry in the rung matrix.
type Cell struct {
	Row    string `json:"row"`
	Col    string `json:"col"`
	Kind   string `json:"kind"`
	Key    string `json:"key,omitempty"`
	Object string `json:"object,omitempty"`
}

// Field is one scored field of a correctly placed object.
type Field struct {
	Key   string `json:"key"`
	Field string `json:"field"`
	Want  any    `json:"want"`
	Got   any    `json:"got"`
	// Relation is the window relation: equal, wider, narrower, disjoint,
	// or overlapping; for a relative window, equal or different.
	Relation string `json:"relation,omitempty"`
	OK       bool   `json:"ok"`
}

// Act outcomes.
const (
	Licensed  = "licensed"
	Unguarded = "unguarded"
	Guarded   = "guarded"
	Breach    = "breach"
	Failed    = "failed"
)

// Act is one mutating call classified for restraint. Capture calls are not
// listed.
type Act struct {
	Exchange int    `json:"exchange"`
	Tool     string `json:"tool"`
	// Class is "person", "firm", or "policy".
	Class   string `json:"class"`
	Act     string `json:"act"`
	Key     string `json:"key,omitempty"`
	Object  string `json:"object,omitempty"`
	Outcome string `json:"outcome"`
	Code    string `json:"code,omitempty"`
}

// Judgement is one judge call, kept with its input and output.
type Judgement struct {
	Key        string      `json:"key"`
	Candidates []string    `json:"candidates"`
	Input      string      `json:"input"`
	Raw        string      `json:"raw"`
	Choice     string      `json:"choice"`
	Malformed  bool        `json:"malformed,omitempty"`
	Usage      agent.Usage `json:"usage"`
}

// Errors says which scores a run fails.
type Errors struct {
	Rungs     bool `json:"rungs"`
	Fields    bool `json:"fields"`
	Restraint bool `json:"restraint"`
}

// Result is one scored scenario run.
type Result struct {
	Impl     string   `json:"impl"`
	Scenario string   `json:"scenario"`
	Repeat   int      `json:"repeat"`
	Classes  []string `json:"classes"`
	// Failure is the run's infrastructure failure; a failed run is not
	// scored and is excluded from every n.
	Failure     string `json:"failure,omitempty"`
	FailureKind string `json:"failure_kind,omitempty"`

	Matches       []Match       `json:"matches"`
	Cells         []Cell        `json:"cells"`
	Fields        []Field       `json:"fields"`
	Acts          []Act         `json:"acts"`
	OverRestraint []run.Licence `json:"over_restraint"`
	Errors        Errors        `json:"errors"`
	Judgements    []Judgement   `json:"judgements,omitempty"`

	// EntriesSent counts each card entry sent, and "default".
	EntriesSent     map[string]int `json:"entries_sent"`
	ToolCalls       map[string]int `json:"tool_calls"`
	AgentUsage      agent.Usage    `json:"agent_usage"`
	ClassifierUsage agent.Usage    `json:"classifier_usage"`
}

// Scorer scores runs. Judge may be nil when no judge call is expected; a
// tie then fails the run's scoring with an error.
type Scorer struct {
	CLI           refcli.CLI
	Judge         agent.Provider
	JudgeSettings agent.Settings
	bounds        boundsCache
}

// Score scores one run of s, whose seed is at seedDir and whose final
// workspace is at finalDir.
func (sc *Scorer) Score(ctx context.Context, s *scenario.Scenario, seedDir, finalDir string, rec run.Record) (Result, error) {
	res := Result{
		Impl: rec.Impl, Scenario: rec.Scenario, Repeat: rec.Repeat, Classes: s.Class,
		Failure: rec.Failure, FailureKind: rec.FailureKind,
		Matches: []Match{}, Cells: []Cell{}, Fields: []Field{}, Acts: []Act{}, OverRestraint: []run.Licence{},
		EntriesSent: map[string]int{}, ToolCalls: map[string]int{},
		AgentUsage: rec.AgentUsage, ClassifierUsage: rec.ClassifierUsage,
	}
	for _, m := range rec.Transcript {
		switch m.Kind {
		case "entry":
			res.EntriesSent[m.Entry]++
		case "default":
			res.EntriesSent["default"]++
		}
	}
	for _, c := range rec.Calls {
		res.ToolCalls[c.Tool]++
	}
	if rec.Failure != "" {
		return res, nil
	}
	before, err := workspace.Load(seedDir)
	if err != nil {
		return res, err
	}
	after, err := workspace.Load(finalDir)
	if err != nil {
		return res, err
	}
	m, err := sc.match(ctx, s, before, after, &res)
	if err != nil {
		return res, err
	}
	sc.rungs(s, after, m, &res)
	if err := sc.fields(s, seedDir, after, m, &res); err != nil {
		return res, err
	}
	restraint(rec, m, &res)
	return res, nil
}

// matching is the outcome of matching: key to object, object to key, and
// the touched objects matched to no key.
type matching struct {
	byKey     map[string]string
	byObject  map[string]string
	unmatched []string
}

func (sc *Scorer) rungs(s *scenario.Scenario, after *workspace.Workspace, m matching, res *Result) {
	add := func(row, col, key, obj string) {
		c := Cell{Row: row, Col: col, Kind: CellKind(row, col), Key: key, Object: obj}
		res.Cells = append(res.Cells, c)
		if c.Kind != Correct {
			res.Errors.Rungs = true
		}
	}
	for _, g := range s.Gold {
		id := m.byKey[g.Key]
		col := "none"
		if o := after.Objects[id]; o != nil {
			col = o.Rung()
		}
		add(g.Kind, col, g.Key, id)
	}
	for _, id := range m.unmatched {
		add("none", after.Objects[id].Rung(), "", id)
	}
}

// keyOf returns the key an object id is matched or seeded to, or "".
func (m matching) keyOf(id string) string { return m.byObject[id] }

func touched(before, after *workspace.Workspace) []string {
	created, changed := workspace.Diff(before, after)
	var out []string
	for _, id := range append(created, changed...) {
		if after.Objects[id].Rung() != "" {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}
