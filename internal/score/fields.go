package score

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/refcli"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/workspace"
)

// fields compares the fields each gold entry lists against its matched
// object, for pairs whose rung is correct. Windows are bounded under the
// resolver context of the seed at seedDir.
func (sc *Scorer) fields(s *scenario.Scenario, seedDir string, after *workspace.Workspace, m matching, res *Result) error {
	for _, g := range s.Gold {
		o := after.Objects[m.byKey[g.Key]]
		if o == nil || o.Rung() != g.Kind {
			continue
		}
		names := make([]string, 0, len(g.Fields))
		for n := range g.Fields {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, name := range names {
			want := g.Fields[name]
			f := Field{Key: g.Key, Field: name, Want: want, Got: o.Fields[name]}
			switch name {
			case "window":
				rel, err := sc.windowRelation(s, seedDir, want, o.Fields["window"], m)
				if err != nil {
					return fmt.Errorf("%s key %s: %w", s.ID, g.Key, err)
				}
				f.Relation, f.OK = rel, rel == "equal"
			case "serves":
				f.Got = o.ServesIDs()
				f.OK = true
				for _, k := range scenario.ServesKeys(want) {
					target := m.byKey[k]
					if target == "" {
						target = s.Keys[k]
					}
					if target == "" || !after.Reaches(o.ID, target) {
						f.OK = false
					}
				}
			case "placed":
				f.Got = o.Has("placement")
				f.OK = want == f.Got
			case "retired":
				f.Got = o.RetiredKind()
				f.OK = want == f.Got
			default:
				f.OK = equalValues(name, want, o.Fields[name])
			}
			if !f.OK {
				res.Errors.Fields = true
			}
			res.Fields = append(res.Fields, f)
		}
	}
	return nil
}

// equalValues compares two field values after normalisation: durations to
// the format's canonical form, set-valued lists sorted, numbers as numbers.
func equalValues(name string, want, got any) bool {
	return reflect.DeepEqual(normalise(name, want), normalise(name, got))
}

var durationRE = regexp.MustCompile(`^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)W)?(?:(\d+)D)?(?:T(?:([\d.]+)H)?(?:([\d.]+)M)?(?:([\d.]+)S)?)?$`)

// NormaliseDuration writes an ISO 8601 duration as the format normalises it
// for hashing: weeks folded into days, the time part re-expressed from its
// total seconds, zero components dropped, a zero duration as P0D. A string
// that is not a duration is returned unchanged.
func NormaliseDuration(s string) string {
	m := durationRE.FindStringSubmatch(s)
	if m == nil || s == "P" || s == "PT" {
		return s
	}
	n := func(i int) float64 {
		if m[i] == "" {
			return 0
		}
		v, _ := strconv.ParseFloat(m[i], 64)
		return v
	}
	years, months, days := int(n(1)), int(n(2)), int(n(3))*7+int(n(4))
	secs := int(math.Round(n(5)*3600 + n(6)*60 + n(7)))
	var b strings.Builder
	b.WriteString("P")
	if years > 0 {
		fmt.Fprintf(&b, "%dY", years)
	}
	if months > 0 {
		fmt.Fprintf(&b, "%dM", months)
	}
	if days > 0 {
		fmt.Fprintf(&b, "%dD", days)
	}
	if secs > 0 {
		b.WriteString("T")
		if h := secs / 3600; h > 0 {
			fmt.Fprintf(&b, "%dH", h)
		}
		if mm := secs % 3600 / 60; mm > 0 {
			fmt.Fprintf(&b, "%dM", mm)
		}
		if ss := secs % 60; ss > 0 {
			fmt.Fprintf(&b, "%dS", ss)
		}
	}
	if b.Len() == 1 {
		return "P0D"
	}
	return b.String()
}

func normalise(name string, v any) any {
	switch t := v.(type) {
	case string:
		return NormaliseDuration(t)
	case int:
		return float64(t)
	case map[string]any:
		out := map[string]any{}
		for k, e := range t {
			out[k] = normalise(k, e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normalise(name, e)
		}
		sort.Slice(out, func(i, j int) bool {
			a, _ := json.Marshal(out[i])
			b, _ := json.Marshal(out[j])
			return string(a) < string(b)
		})
		return out
	case nil:
		return nil
	default:
		return v
	}
}

// windowRelation compares the run's window with the gold window as the
// relation between the sets of instants each admits, under the seed's
// resolver context, computed by the pinned reference CLI. A relative gold
// window is compared on target and relation.
func (sc *Scorer) windowRelation(s *scenario.Scenario, wsDir string, want, got any, m matching) (string, error) {
	w, _ := want.(map[string]any)
	g, _ := got.(map[string]any)
	if rel, ok := w["relative"].(map[string]any); ok {
		gr, _ := g["relative"].(map[string]any)
		target, _ := gr["target"].(string)
		key := m.keyOf(target)
		if key == "" {
			key = target
		}
		if key == rel["target"] && gr["relation"] == rel["relation"] {
			return "equal", nil
		}
		return "different", nil
	}
	from, to := s.Now.AddDate(-1, 0, 0), s.Now.AddDate(2, 0, 0)
	wantIv, err := sc.windowIntervals(wsDir, w, s.Now, from, to)
	if err != nil {
		return "", err
	}
	gotIv, err := sc.windowIntervals(wsDir, g, s.Now, from, to)
	if err != nil {
		return "", err
	}
	return Relation(gotIv, wantIv), nil
}

func (sc *Scorer) windowIntervals(wsDir string, w map[string]any, now, from, to time.Time) ([]refcli.Interval, error) {
	cal, _ := w["calendar"].(string)
	clock, _ := w["clock"].(string)
	if cal == "" && clock == "" {
		return []refcli.Interval{{Start: from, End: to}}, nil
	}
	return sc.bounds.get(sc.CLI, wsDir, cal, clock, now, from, to)
}

// Relation is the relation of got to want as sets of instants: equal,
// narrower (got admits less), wider (got admits more), disjoint, or
// overlapping (each admits instants the other does not).
func Relation(got, want []refcli.Interval) string {
	got, want = merge(got), merge(want)
	gotIn, wantIn := covers(want, got), covers(got, want)
	switch {
	case gotIn && wantIn:
		return "equal"
	case gotIn:
		return "narrower"
	case wantIn:
		return "wider"
	case !intersects(got, want):
		return "disjoint"
	default:
		return "overlapping"
	}
}

func merge(iv []refcli.Interval) []refcli.Interval {
	out := append([]refcli.Interval(nil), iv...)
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	var merged []refcli.Interval
	for _, x := range out {
		if !x.End.After(x.Start) {
			continue
		}
		if n := len(merged); n > 0 && !x.Start.After(merged[n-1].End) {
			if x.End.After(merged[n-1].End) {
				merged[n-1].End = x.End
			}
			continue
		}
		merged = append(merged, x)
	}
	return merged
}

// covers reports whether every instant in inner lies in outer; both merged.
func covers(outer, inner []refcli.Interval) bool {
	for _, x := range inner {
		ok := false
		for _, o := range outer {
			if !x.Start.Before(o.Start) && !x.End.After(o.End) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func intersects(a, b []refcli.Interval) bool {
	for _, x := range a {
		for _, y := range b {
			if x.Start.Before(y.End) && y.Start.Before(x.End) {
				return true
			}
		}
	}
	return false
}

// boundsCache memoises reference CLI bounds calls.
type boundsCache struct {
	mu sync.Mutex
	m  map[string][]refcli.Interval
}

func (c *boundsCache) get(cli refcli.CLI, ws, cal, clock string, now, from, to time.Time) ([]refcli.Interval, error) {
	key := strings.Join([]string{ws, cal, clock, now.String(), from.String(), to.String()}, "\x00")
	c.mu.Lock()
	if iv, ok := c.m[key]; ok {
		c.mu.Unlock()
		return iv, nil
	}
	c.mu.Unlock()
	iv, err := cli.Bounds(ws, cal, clock, now, from, to)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	if c.m == nil {
		c.m = map[string][]refcli.Interval{}
	}
	c.m[key] = iv
	c.mu.Unlock()
	return iv, nil
}
