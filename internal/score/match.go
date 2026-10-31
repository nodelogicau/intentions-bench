package score

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/workspace"
	"github.com/nodelogicau/intentions-bench/prompts"
)

// match binds every gold key to at most one object and every object to at
// most one key. A seed key is bound to its seed object. A new key is matched
// among the objects the run created or changed: first among those of the
// key's kind, then among the rest, preferring the same subject and then the
// most scored fields equal. Where several candidates tie, the judge chooses
// by title and description.
func (sc *Scorer) match(ctx context.Context, s *scenario.Scenario, before, after *workspace.Workspace, res *Result) (matching, error) {
	m := matching{byKey: map[string]string{}, byObject: map[string]string{}}
	for k, id := range s.Keys {
		m.byObject[id] = k
	}
	var pending []scenario.GoldEntry
	for _, g := range s.Gold {
		if id, ok := s.Keys[g.Key]; ok {
			m.byKey[g.Key] = id
			res.Matches = append(res.Matches, Match{Key: g.Key, Object: id, By: "seed"})
			continue
		}
		pending = append(pending, g)
	}
	var free []string
	for _, id := range touched(before, after) {
		if _, seeded := m.byObject[id]; !seeded {
			free = append(free, id)
		}
	}
	take := func(g scenario.GoldEntry, id, by string) {
		m.byKey[g.Key], m.byObject[id] = id, g.Key
		free = slices.DeleteFunc(free, func(x string) bool { return x == id })
		res.Matches = append(res.Matches, Match{Key: g.Key, Object: id, By: by})
	}
	for _, sameKind := range []bool{true, false} {
		var left []scenario.GoldEntry
		for _, g := range pending {
			var cands []string
			for _, id := range free {
				if !sameKind || after.Objects[id].Rung() == g.Kind {
					cands = append(cands, id)
				}
			}
			best := bestByStructure(g, cands, after)
			switch len(best) {
			case 0:
				left = append(left, g)
			case 1:
				take(g, best[0], "structure")
			default:
				id, err := sc.judge(ctx, g, best, after, res)
				if err != nil {
					return m, err
				}
				take(g, id, "judge")
			}
		}
		pending = left
	}
	for _, g := range pending {
		res.Matches = append(res.Matches, Match{Key: g.Key})
	}
	m.unmatched = free
	return m, nil
}

// bestByStructure returns the candidates that tie best on subject and then
// on the number of scored fields that compare equal by their written form.
func bestByStructure(g scenario.GoldEntry, cands []string, ws *workspace.Workspace) []string {
	type score struct{ subject, fields int }
	var best []string
	var top score
	for _, id := range cands {
		o := ws.Objects[id]
		sc := score{}
		if want, ok := g.Fields["subject"]; !ok || want == o.Str("subject") {
			sc.subject = 1
		}
		for name, want := range g.Fields {
			switch name {
			case "serves", "subject":
				continue
			case "placed":
				if want == o.Has("placement") {
					sc.fields++
				}
			case "retired":
				if want == o.RetiredKind() {
					sc.fields++
				}
			default:
				if equalValues(name, want, o.Fields[name]) {
					sc.fields++
				}
			}
		}
		switch {
		case best == nil || sc.subject > top.subject || (sc.subject == top.subject && sc.fields > top.fields):
			best, top = []string{id}, sc
		case sc == top:
			best = append(best, id)
		}
	}
	return best
}

// judge asks the judge model which of the tied candidates is the gold key.
func (sc *Scorer) judge(ctx context.Context, g scenario.GoldEntry, cands []string, ws *workspace.Workspace, res *Result) (string, error) {
	if sc.Judge == nil {
		return "", fmt.Errorf("key %s ties between %v and needs the judge, but no judge model is available (set ANTHROPIC_API_KEY)", g.Key, cands)
	}
	input := JudgeInput(g, cands, ws)
	raw, usage, err := agent.Complete(ctx, sc.Judge, sc.JudgeSettings, "", input)
	if err != nil {
		return "", fmt.Errorf("judge for key %s: %w", g.Key, err)
	}
	j := Judgement{Key: g.Key, Candidates: cands, Input: input, Raw: raw, Usage: usage}
	choice := strings.Trim(strings.TrimSpace(raw), "`'\".")
	if !slices.Contains(cands, choice) {
		j.Malformed = true
		choice = cands[0]
	}
	j.Choice = choice
	res.Judgements = append(res.Judgements, j)
	return choice, nil
}

// JudgeInput renders the judge prompt for a key and its tied candidates.
func JudgeInput(g scenario.GoldEntry, cands []string, ws *workspace.Workspace) string {
	expected, _ := json.Marshal(map[string]any{"kind": g.Kind, "title": g.Title, "fields": g.Fields})
	var lines []string
	for _, id := range cands {
		o := ws.Objects[id]
		c, _ := json.Marshal(map[string]any{"id": id, "rung": o.Rung(), "title": o.Str("title"), "description": o.Str("description")})
		lines = append(lines, string(c))
	}
	return strings.NewReplacer("{expected}", string(expected), "{candidates}", strings.Join(lines, "\n")).Replace(prompts.Get(prompts.Judge))
}
