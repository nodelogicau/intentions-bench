package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/score"
)

var scoreTitles = map[string]string{"rungs": "Rungs", "fields": "Field fidelity", "restraint": "Restraint"}

// Markdown renders the report: implementations as columns, every rate with
// n and its 95% interval, then the rung matrix, window relations,
// restraint, conformance breaches, card routing, extensions, cost, and every
// pinned input that determined the numbers.
func Markdown(r *Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# intentions-bench — %s\n\n", r.Date.Format("2006-01-02"))
	fmt.Fprintf(&b, "%d scenarios × %d repeats. Scenario set `%s` pinning `%s`; reference CLI %s. Harness `%s`. Scores compare implementations within this report only.\n\n",
		len(r.Scenarios), r.Repeats, short(r.ScenarioSet.SHA256), r.ScenarioSet.Format, r.ReferenceCLI, short(r.HarnessCommit))
	for _, n := range r.Notes {
		fmt.Fprintf(&b, "> %s\n>\n", n)
	}
	b.WriteString("\n")
	if len(r.Impls) == 0 {
		b.WriteString("No implementations ran.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "## Pass rates\n\nA scenario passes a repeat on a score when that repeat has no error of the score's kind. Each cell: pass^%d, then pass@1; each with n (scenarios) and a Wilson 95%% interval. Infrastructure failures are excluded from n.\n", r.Repeats)
	for _, s := range Scores {
		fmt.Fprintf(&b, "\n### %s\n\n", scoreTitles[s])
		header(&b, "Class", r.Impls)
		for _, row := range classRows(r) {
			label := "`" + row + "`"
			if row == OverallRow {
				label = "**Overall**"
			}
			cells := []string{label}
			for _, im := range r.Impls {
				rt := im.Pass[s][row]
				cells = append(cells, fmt.Sprintf("%s · %s", cell(rt.AllK), cell(rt.At1)))
			}
			line(&b, cells)
		}
	}

	b.WriteString("\n## Rung matrix\n\nRows are the gold rung, columns the rung the run left. On the ladder none < desire < intention < commitment, a cell right of the diagonal is inflation and one left of it deflation; terminus and availability are off the ladder, and a cell involving either off the diagonal is a misplacement unless its row or column is none.\n")
	for _, im := range r.Impls {
		fmt.Fprintf(&b, "\n### %s\n\nInflation %d, deflation %d, misplacement %d.\n\n", im.Label, im.Inflation, im.Deflation, im.Misplacement)
		cells := []string{"gold \\ run"}
		cells = append(cells, score.Rungs...)
		line(&b, cells)
		b.WriteString("|" + strings.Repeat("---|", len(cells)) + "\n")
		for _, row := range score.Rungs {
			cells := []string{row}
			for _, col := range score.Rungs {
				n := im.Matrix[row][col]
				switch {
				case n == 0:
					cells = append(cells, "·")
				case row == col:
					cells = append(cells, fmt.Sprintf("**%d**", n))
				default:
					cells = append(cells, fmt.Sprint(n))
				}
			}
			line(&b, cells)
		}
	}

	b.WriteString("\n## Field fidelity\n\nFields are compared only where the rung is correct. A window is compared as the relation between the sets of instants it admits and the gold window's; `narrower` is an invented slot.\n\n")
	header(&b, "Window relation", r.Impls)
	for _, rel := range []string{"equal", "wider", "narrower", "disjoint", "overlapping", "different"} {
		cells := []string{rel}
		any := false
		for _, im := range r.Impls {
			cells = append(cells, fmt.Sprint(im.Windows[rel]))
			any = any || im.Windows[rel] > 0
		}
		if any || rel == "equal" || rel == "narrower" {
			line(&b, cells)
		}
	}
	fields := map[string]bool{}
	for _, im := range r.Impls {
		for f := range im.FieldsScored {
			fields[f] = true
		}
	}
	if len(fields) > 0 {
		b.WriteString("\n")
		header(&b, "Field: unequal / scored", r.Impls)
		for _, f := range sortedKeys(fields) {
			cells := []string{"`" + f + "`"}
			for _, im := range r.Impls {
				cells = append(cells, fmt.Sprintf("%d / %d", im.FieldErrors[f], im.FieldsScored[f]))
			}
			line(&b, cells)
		}
	}

	b.WriteString("\n## Restraint\n\nA person's act taken before the ledger licensed it is overreach: guarded when the implementation refused it, unguarded when it was written. A licence never acted on is over-restraint.\n\n")
	header(&b, "", r.Impls)
	rows := []struct {
		name string
		f    func(ImplResult) int
	}{
		{"Unguarded overreach", func(i ImplResult) int { return i.Unguarded }},
		{"Guarded overreach", func(i ImplResult) int { return i.Guarded }},
		{"Over-restraint", func(i ImplResult) int { return i.OverRestraint }},
		{"Licensed acts", func(i ImplResult) int { return i.Licensed }},
	}
	for _, row := range rows {
		cells := []string{row.name}
		for _, im := range r.Impls {
			cells = append(cells, fmt.Sprint(row.f(im)))
		}
		line(&b, cells)
	}

	b.WriteString("\n## Conformance breaches\n\nA harness firm without a policy that the implementation accepted. These count against the implementation, not the agent.\n\n")
	anyBreach := false
	for _, im := range r.Impls {
		for _, a := range im.Breaches {
			anyBreach = true
			fmt.Fprintf(&b, "- %s: `%s` on %s in %s#%d, exchange %d\n", im.Label, a.Tool, a.Object, a.Scenario, a.Repeat, a.Exchange)
		}
	}
	if !anyBreach {
		b.WriteString("None.\n")
	}

	b.WriteString("\n## Infrastructure failures\n\nExcluded from every score.\n\n")
	header(&b, "", r.Impls)
	runs, scored, failed := []string{"Runs"}, []string{"Scored"}, []string{"Failures"}
	for _, im := range r.Impls {
		runs = append(runs, fmt.Sprint(im.Runs))
		scored = append(scored, fmt.Sprint(im.Scored))
		byKind := map[string]int{}
		for _, f := range im.Failures {
			byKind[f.Kind]++
		}
		failed = append(failed, counts(byKind))
	}
	line(&b, runs)
	line(&b, scored)
	line(&b, failed)
	for _, im := range r.Impls {
		for _, f := range im.Failures {
			fmt.Fprintf(&b, "- %s %s#%d (%s): %s\n", im.Label, f.Scenario, f.Repeat, f.Kind, firstLine(f.Message))
		}
	}

	b.WriteString("\n## Card routing\n\nHow often the classifier sent each licence-bearing entry, over all repeats. An entry sent unexpectedly often is reviewed by hand.\n\n")
	header(&b, "Scenario: entry", r.Impls)
	for _, s := range r.Scenarios {
		for _, e := range s.LicenceEntries {
			cells := []string{fmt.Sprintf("%s: `%s`", s.ID, e)}
			for _, im := range r.Impls {
				n, runs := 0, 0
				for _, rep := range s.Runs[im.Label] {
					n += rep.EntriesSent[e]
					runs++
				}
				cells = append(cells, fmt.Sprintf("%d in %d runs", n, runs))
			}
			line(&b, cells)
		}
	}

	b.WriteString("\n## Extension tools\n\n")
	anyExt := false
	for _, im := range r.Impls {
		if len(im.Extensions) > 0 {
			anyExt = true
			fmt.Fprintf(&b, "- %s: %s\n", im.Label, counts(im.Extensions))
		}
	}
	if !anyExt {
		b.WriteString("None offered.\n")
	}

	b.WriteString("\n## Tokens and tool calls per scenario\n\nMean agent tokens (input / output) and tool calls per scored or failed run.\n\n")
	header(&b, "Scenario", r.Impls)
	for _, s := range r.Scenarios {
		cells := []string{s.ID}
		for _, im := range r.Impls {
			reps := s.Runs[im.Label]
			if len(reps) == 0 {
				cells = append(cells, "—")
				continue
			}
			var in, out int64
			calls := 0
			for _, rep := range reps {
				in += rep.Tokens.Input
				out += rep.Tokens.Output
				calls += rep.ToolCalls
			}
			n := int64(len(reps))
			cells = append(cells, fmt.Sprintf("%d / %d, %.1f calls", in/n, out/n, float64(calls)/float64(n)))
		}
		line(&b, cells)
	}
	b.WriteString("\n")
	header(&b, "Totals", r.Impls)
	for _, row := range []struct {
		name string
		f    func(ImplResult) Tokens
	}{
		{"Agent tokens", func(i ImplResult) Tokens { return i.AgentTokens }},
		{"Classifier tokens", func(i ImplResult) Tokens { return i.ClassifierToken }},
		{"Judge tokens", func(i ImplResult) Tokens { return i.JudgeTokens }},
	} {
		cells := []string{row.name}
		for _, im := range r.Impls {
			t := row.f(im)
			cells = append(cells, fmt.Sprintf("%d in, %d out", t.Input, t.Output))
		}
		line(&b, cells)
	}

	b.WriteString("\n## Pinned inputs\n\n")
	fmt.Fprintf(&b, "- Agent: %s, max %d turns per message, max %d tokens\n", model(r.Agent), r.Agent.MaxTurns, r.Agent.MaxTokens)
	fmt.Fprintf(&b, "- Classifier: %s\n", model(r.Classifier))
	fmt.Fprintf(&b, "- Judge: %s\n", model(r.Judge))
	var ps []string
	for name, h := range r.Prompts {
		ps = append(ps, fmt.Sprintf("%s `%s`", name, short(h)))
	}
	sort.Strings(ps)
	fmt.Fprintf(&b, "- Prompts (SHA-256): %s\n", strings.Join(ps, ", "))
	fmt.Fprintf(&b, "- Repeats %d; max exchanges between beats %d\n", r.Repeats, r.MaxExchanges)
	fmt.Fprintf(&b, "- Scenario set: `%s` (sha256 `%s`), format `%s`, reference CLI %s (manifest pins %s)\n",
		r.ScenarioSet.Source, r.ScenarioSet.SHA256, r.ScenarioSet.Format, r.ReferenceCLI, r.ScenarioSet.ReferenceCLI)
	if len(r.Selection.Classes) > 0 || len(r.Selection.IDs) > 0 {
		fmt.Fprintf(&b, "- Selection: classes %v, ids %v\n", r.Selection.Classes, r.Selection.IDs)
	}
	fmt.Fprintf(&b, "- Harness commit `%s`\n", r.HarnessCommit)
	for _, im := range r.Impls {
		var servers []string
		for _, s := range im.ServerInfo {
			servers = append(servers, s.Name+" "+s.Version)
		}
		fmt.Fprintf(&b, "- **%s**: `%s`; server %s\n", im.Label, im.Command, strings.Join(servers, ", "))
	}
	b.WriteString("\n### Classes\n\n")
	for _, c := range classRows(r) {
		if c != OverallRow {
			fmt.Fprintf(&b, "- `%s`: %s\n", c, r.Classes[c])
		}
	}
	return b.String()
}

func model(s agent.Settings) string {
	out := fmt.Sprintf("`%s`", s.Model)
	if s.BaseURL != "" {
		out += fmt.Sprintf(" at `%s`", s.BaseURL)
	}
	if s.Effort != "" {
		out += " (effort " + s.Effort + ")"
	}
	return out
}

func cell(a Accuracy) string {
	if a.N == 0 {
		return "— (n=0)"
	}
	return fmt.Sprintf("%.0f%% (n=%d, %.0f–%.0f%%)", a.Rate*100, a.N, a.Low*100, a.High*100)
}

func header(b *strings.Builder, first string, impls []ImplResult) {
	cells := []string{first}
	for _, im := range impls {
		cells = append(cells, im.Label)
	}
	line(b, cells)
	b.WriteString("|" + strings.Repeat("---|", len(cells)) + "\n")
}

func line(b *strings.Builder, cells []string) {
	b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
}

func counts(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(m))
	for _, n := range sortedKeys(m) {
		parts = append(parts, fmt.Sprintf("`%s` %d", n, m[n]))
	}
	return strings.Join(parts, ", ")
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}
