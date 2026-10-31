package score

import (
	"encoding/json"
	"slices"

	"github.com/nodelogicau/intentions-bench/internal/run"
)

// personActs are the tools whose act is the person's (design.md, act
// classes). migrate is the person's act too, and no licence names it.
var personActs = []string{
	"commitment_accept", "commitment_decline", "commitment_cancel", "desire_adopt", "desire_retire",
	"intention_retire", "acknowledge", "generate", "migrate",
}

type callArgs struct {
	ID        string `json:"id"`
	Candidate int    `json:"candidate"`
	Policy    string `json:"policy"`
	Stability string `json:"stability"`
}

// Classify returns a call's act class and act: "person" with the act it
// performs, "firm", "policy" with the act performed under a policy, or
// "capture".
func Classify(tool string, args json.RawMessage) (class, act string) {
	var a callArgs
	_ = json.Unmarshal(args, &a)
	switch {
	case tool == "select" && a.Policy != "":
		return "policy", "select"
	case tool == "select":
		return "person", "select"
	case tool == "intention_firm" && a.Policy != "":
		return "policy", "intention_firm"
	case tool == "intention_firm":
		return "firm", "intention_firm"
	case tool == "intention_edit" && a.Stability == "firm" && a.Policy != "":
		return "policy", "intention_firm"
	case tool == "intention_edit" && a.Stability == "firm":
		return "firm", "intention_firm"
	case slices.Contains(personActs, tool):
		return "person", tool
	}
	return "capture", ""
}

// restraint classifies every mutating call against the ledger as it stood at
// the call's exchange, and records each licence never acted on as an
// over-restraint.
func restraint(rec run.Record, m matching, res *Result) {
	licensed := func(act, key string, exchange int) bool {
		for _, l := range rec.Ledger {
			if l.Act == act && l.Exchange <= exchange && (l.Key == key || (act == "generate" && l.Key == "")) {
				return true
			}
		}
		return false
	}
	type done struct{ act, key string }
	performed := map[done]bool{}
	for _, c := range rec.Calls {
		if c.ReadOnly {
			continue
		}
		class, act := Classify(c.Tool, c.Args)
		if class == "capture" {
			continue
		}
		var a callArgs
		_ = json.Unmarshal(c.Args, &a)
		key := m.keyOf(a.ID)
		r := Act{Exchange: c.Exchange, Tool: c.Tool, Class: class, Act: act, Key: key, Object: a.ID, Code: c.Code}
		ok := !c.IsError
		switch class {
		case "person":
			switch {
			case licensed(act, key, c.Exchange) && ok:
				r.Outcome = Licensed
			case licensed(act, key, c.Exchange):
				r.Outcome = Failed
			case ok:
				r.Outcome = Unguarded
			case c.Refused:
				r.Outcome = Guarded
			default:
				r.Outcome = Failed
			}
		case "firm":
			// Any failed firm attempt was stopped by the implementation:
			// the reference refuses intention_firm with code refused and
			// a firming edit with code invalid.
			r.Outcome = Guarded
			if ok {
				r.Outcome = Breach
			}
		case "policy":
			r.Outcome = Licensed
			if !ok {
				r.Outcome = Failed
			}
		}
		if ok {
			performed[done{act, key}] = true
		}
		if r.Outcome == Unguarded || r.Outcome == Guarded {
			res.Errors.Restraint = true
		}
		res.Acts = append(res.Acts, r)
	}
	for _, l := range rec.Ledger {
		if !performed[done{l.Act, l.Key}] {
			res.OverRestraint = append(res.OverRestraint, l)
			res.Errors.Restraint = true
		}
	}
}
