// Package scenario reads a scenario set: its manifest, scenario files, seed
// workspaces with their key maps, and gold files. It refuses a set that does
// not parse or breaks a structural rule, naming the file and the field.
// Checks that need the reference CLI live in package refcli.
package scenario

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nodelogicau/intentions-bench/internal/yamlutil"
)

// Acts a licence may name. intention_firm is refused separately while the
// format reserves firming to the person's own act outside a harness.
var Acts = []string{
	"select", "commitment_accept", "commitment_decline", "commitment_cancel",
	"desire_adopt", "desire_retire", "intention_retire", "acknowledge", "generate",
}

// Kinds a gold entry may name.
var Kinds = []string{"desire", "intention", "terminus", "availability", "commitment"}

// GoldFields are the fields a gold entry may list for scoring, beyond key,
// kind, title and adopted_as.
var GoldFields = []string{
	"window", "duration", "capacity", "stability", "subject", "activity", "activities",
	"location", "parties", "serves", "cadence", "auto_select", "auto_firm", "retired", "placed",
}

// FirmAssentQuestion names the open question a firm licence depends on.
const FirmAssentQuestion = "design.md, Open Questions: assent to firm versus assent to select (class restraint/firm-assent)"

// Manifest is a scenario set's manifest.yaml.
type Manifest struct {
	// Format is the format version every seed pins, e.g. intentions/0.2.
	Format string `yaml:"format"`
	// ReferenceCLI is the version of the reference CLI that checks the set.
	ReferenceCLI string `yaml:"reference_cli"`
	// Classes describes every class label in use, one line each.
	Classes map[string]string `yaml:"classes"`
}

// Licence is an act the person licenses on a gold or seed key.
type Licence struct {
	Act string `yaml:"act" json:"act"`
	Key string `yaml:"key" json:"key"`
}

// Beat is what the person says, in order.
type Beat struct {
	Say      string    `yaml:"say"`
	Licenses []Licence `yaml:"licenses"`
}

// Entry is a fixed answer on the card.
type Entry struct {
	ID       string    `yaml:"id"`
	When     string    `yaml:"when"`
	Say      string    `yaml:"say"`
	Licenses []Licence `yaml:"licenses"`
}

// Scenario is one scenario file.
type Scenario struct {
	ID      string   `yaml:"id"`
	Class   []string `yaml:"class"`
	NowRaw  string   `yaml:"now"`
	Seed    string   `yaml:"seed"`
	Beats   []Beat   `yaml:"beats"`
	Card    []Entry  `yaml:"card"`
	Default string   `yaml:"default"`
	GoldRaw string   `yaml:"gold"`

	// File is the scenario's path relative to the set root.
	File string `yaml:"-"`
	// Now is NowRaw parsed.
	Now time.Time `yaml:"-"`
	// Gold is the parsed gold file.
	Gold []GoldEntry `yaml:"-"`
	// Keys is the seed's key map.
	Keys map[string]string `yaml:"-"`
}

// Entry returns the card entry with id, if any.
func (s *Scenario) Entry(id string) (Entry, bool) {
	for _, e := range s.Card {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// GoldKey returns the gold entry for key, if any.
func (s *Scenario) GoldKey(key string) (GoldEntry, bool) {
	for _, g := range s.Gold {
		if g.Key == key {
			return g, true
		}
	}
	return GoldEntry{}, false
}

// HasClass reports whether the scenario carries class.
func (s *Scenario) HasClass(c string) bool { return slices.Contains(s.Class, c) }

// GoldEntry is one key in a gold file: an object a correct run leaves beyond
// the seed, or a seed object it changes.
type GoldEntry struct {
	Key  string `yaml:"key" json:"key"`
	Kind string `yaml:"kind" json:"kind"`
	// Title describes the object for the judge, which aligns objects to
	// keys only when structure ties. It is never scored.
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	// AdoptedAs names the key of the intention a desire was adopted as;
	// it is needed to build the gold workspace and is not scored.
	AdoptedAs string `yaml:"adopted_as,omitempty" json:"adopted_as,omitempty"`
	// Fields are the scored fields, by name.
	Fields map[string]any `yaml:"-" json:"fields"`
}

// UnmarshalYAML keeps every key other than key, kind, title and adopted_as
// as a scored field.
func (g *GoldEntry) UnmarshalYAML(n *yaml.Node) error {
	v, err := yamlutil.Plain(n)
	if err != nil {
		return err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("line %d: a gold entry is a mapping", n.Line)
	}
	g.Fields = map[string]any{}
	for k, v := range m {
		switch k {
		case "key", "kind", "title", "adopted_as":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("line %d: %s is not a string", n.Line, k)
			}
			switch k {
			case "key":
				g.Key = s
			case "kind":
				g.Kind = s
			case "title":
				g.Title = s
			case "adopted_as":
				g.AdoptedAs = s
			}
		default:
			g.Fields[k] = v
		}
	}
	return nil
}

// Set is a scenario set.
type Set struct {
	Root      string
	Manifest  Manifest
	Scenarios []*Scenario
}

// Problems is every reason a set was refused.
type Problems []string

func (p Problems) Error() string {
	return fmt.Sprintf("scenario set refused (%d problems):\n  %s", len(p), strings.Join(p, "\n  "))
}

func (p *Problems) add(format string, args ...any) { *p = append(*p, fmt.Sprintf(format, args...)) }

// Load reads the set at root: manifest.yaml and scenarios/*.yaml, with the
// seeds and gold files they name. It returns Problems when the set breaks a
// structural rule.
func Load(root string) (*Set, error) {
	set := &Set{Root: root}
	var ps Problems
	if err := decodeFile(root, "manifest.yaml", &set.Manifest); err != nil {
		return nil, err
	}
	m := set.Manifest
	if m.Format == "" {
		ps.add("manifest.yaml: missing format")
	}
	if m.ReferenceCLI == "" {
		ps.add("manifest.yaml: missing reference_cli")
	}
	for c, d := range m.Classes {
		if strings.TrimSpace(d) == "" {
			ps.add("manifest.yaml: class %s has no description", c)
		}
	}
	files, err := filepath.Glob(filepath.Join(root, "scenarios", "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		ps.add("scenarios/: no scenario files")
	}
	seen := map[string]string{}
	seeds := map[string]bool{}
	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		s := &Scenario{File: rel}
		if err := decodeFile(root, rel, s); err != nil {
			ps.add("%v", err)
			continue
		}
		set.checkScenario(s, &ps)
		if s.ID != "" {
			if other, dup := seen[s.ID]; dup {
				ps.add("%s: id %s is also used by %s", rel, s.ID, other)
			}
			seen[s.ID] = rel
		}
		if s.Seed != "" {
			seeds[s.Seed] = true
		}
		set.Scenarios = append(set.Scenarios, s)
	}
	for seed := range seeds {
		set.checkSeedFormat(seed, &ps)
	}
	if len(ps) > 0 {
		sort.Strings(ps)
		return nil, ps
	}
	return set, nil
}

// decodeFile decodes a YAML file strictly, naming the file in any error.
func decodeFile(root, rel string, v any) error {
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return fmt.Errorf("%s: %w", rel, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: does not parse: %w", rel, err)
	}
	return nil
}

func (set *Set) checkScenario(s *Scenario, ps *Problems) {
	f := s.File
	required := []struct {
		field string
		ok    bool
	}{
		{"id", s.ID != ""}, {"class", len(s.Class) > 0}, {"now", s.NowRaw != ""}, {"seed", s.Seed != ""},
		{"beats", len(s.Beats) > 0}, {"default", s.Default != ""}, {"gold", s.GoldRaw != ""},
	}
	for _, r := range required {
		if !r.ok {
			ps.add("%s: missing %s", f, r.field)
		}
	}
	if s.NowRaw != "" {
		t, err := time.Parse(time.RFC3339, s.NowRaw)
		if err != nil || !hasOffset(s.NowRaw) {
			ps.add("%s: now %q is not an RFC 3339 timestamp with offset", f, s.NowRaw)
		}
		s.Now = t
	}
	for _, c := range s.Class {
		if _, ok := set.Manifest.Classes[c]; !ok {
			ps.add("%s: class %s is not in manifest.yaml", f, c)
		}
	}
	for i, b := range s.Beats {
		if strings.TrimSpace(b.Say) == "" {
			ps.add("%s: beats[%d] has no say", f, i)
		}
	}
	ids := map[string]bool{}
	for i, e := range s.Card {
		switch {
		case e.ID == "":
			ps.add("%s: card[%d] has no id", f, i)
		case e.ID == "default" || e.ID == "none":
			ps.add("%s: card entry id %q is reserved for the classifier", f, e.ID)
		case ids[e.ID]:
			ps.add("%s: scenario %s has two card entries with id %s", f, s.ID, e.ID)
		}
		ids[e.ID] = true
		if strings.TrimSpace(e.When) == "" {
			ps.add("%s: card entry %s has no when", f, e.ID)
		}
		if strings.TrimSpace(e.Say) == "" {
			ps.add("%s: card entry %s has no say", f, e.ID)
		}
	}
	if s.Seed != "" {
		keys, err := readKeys(set.Root, s.Seed)
		if err != nil {
			ps.add("%s: seed: %v", f, err)
		}
		s.Keys = keys
	}
	if s.GoldRaw != "" {
		if err := decodeFile(set.Root, s.GoldRaw, &s.Gold); err != nil {
			ps.add("%s: gold: %v", f, err)
		} else {
			checkGold(s, ps)
		}
	}
	known := func(key string) bool {
		_, inSeed := s.Keys[key]
		_, inGold := s.GoldKey(key)
		return inSeed || inGold
	}
	checkLicences := func(where string, ls []Licence) {
		for _, l := range ls {
			switch {
			case l.Act == "intention_firm":
				ps.add("%s: %s licenses intention_firm, which no scenario may license while an open question stands: %s", f, where, FirmAssentQuestion)
			case !slices.Contains(Acts, l.Act):
				ps.add("%s: %s licenses unknown act %q (acts: %s)", f, where, l.Act, strings.Join(Acts, ", "))
			}
			if l.Key == "" && l.Act != "generate" {
				ps.add("%s: %s licenses %s with no key", f, where, l.Act)
			} else if l.Key != "" && !known(l.Key) {
				ps.add("%s: scenario %s: %s licenses key %s, which neither the gold file nor the seed key map defines", f, s.ID, where, l.Key)
			}
		}
	}
	for i, b := range s.Beats {
		checkLicences(fmt.Sprintf("beats[%d]", i), b.Licenses)
	}
	for _, e := range s.Card {
		checkLicences("card entry "+e.ID, e.Licenses)
	}
}

func hasOffset(s string) bool {
	return strings.HasSuffix(s, "Z") || strings.LastIndexAny(s, "+-") > strings.Index(s, "T")
}

func checkGold(s *Scenario, ps *Problems) {
	f := s.GoldRaw
	keys := map[string]bool{}
	for i, g := range s.Gold {
		if g.Key == "" {
			ps.add("%s: entry %d has no key", f, i)
			continue
		}
		if keys[g.Key] {
			ps.add("%s: key %s listed twice", f, g.Key)
		}
		keys[g.Key] = true
		if !slices.Contains(Kinds, g.Kind) {
			ps.add("%s: key %s: kind %q is not one of %s", f, g.Key, g.Kind, strings.Join(Kinds, ", "))
		}
		for name := range g.Fields {
			if !slices.Contains(GoldFields, name) {
				ps.add("%s: key %s: unknown field %s (fields: %s)", f, g.Key, name, strings.Join(GoldFields, ", "))
			}
		}
		if g.AdoptedAs != "" && g.Fields["retired"] != "adopted" {
			ps.add("%s: key %s: adopted_as without retired: adopted", f, g.Key)
		}
		if g.Fields["retired"] == "adopted" && g.AdoptedAs == "" {
			ps.add("%s: key %s: retired: adopted needs adopted_as", f, g.Key)
		}
	}
	isKey := func(k string) bool {
		_, inSeed := s.Keys[k]
		return inSeed || keys[k]
	}
	for _, g := range s.Gold {
		for _, t := range ServesKeys(g.Fields["serves"]) {
			if !isKey(t) {
				ps.add("%s: key %s: serves %s, which neither the gold file nor the seed key map defines", f, g.Key, t)
			}
		}
		if w, ok := g.Fields["window"].(map[string]any); ok {
			if rel, ok := w["relative"].(map[string]any); ok {
				if t, _ := rel["target"].(string); !isKey(t) {
					ps.add("%s: key %s: window.relative.target %q is not a key", f, g.Key, t)
				}
			}
		}
		if g.AdoptedAs != "" && !keys[g.AdoptedAs] {
			ps.add("%s: key %s: adopted_as %s is not a gold key", f, g.Key, g.AdoptedAs)
		}
	}
}

// ServesKeys returns the keys a gold serves field lists: a list of keys, or
// of {key, role} mappings.
func ServesKeys(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, e := range list {
		switch t := e.(type) {
		case string:
			out = append(out, t)
		case map[string]any:
			if k, ok := t["key"].(string); ok {
				out = append(out, k)
			}
		}
	}
	return out
}

func readKeys(root, seed string) (map[string]string, error) {
	keys := map[string]string{}
	rel := filepath.Join(seed, "keys.yaml")
	if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
		return keys, fmt.Errorf("%s: missing", rel)
	}
	if err := decodeFile(root, rel, &keys); err != nil {
		return keys, err
	}
	if keys == nil {
		keys = map[string]string{}
	}
	return keys, nil
}

func (set *Set) checkSeedFormat(seed string, ps *Problems) {
	rel := filepath.Join(seed, "intentions.yaml")
	var cfg struct {
		Format string `yaml:"format"`
	}
	b, err := os.ReadFile(filepath.Join(set.Root, rel))
	if err != nil {
		ps.add("seed %s: no intentions.yaml", seed)
		return
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		ps.add("%s: does not parse: %v", rel, err)
		return
	}
	if cfg.Format != set.Manifest.Format {
		ps.add("seed %s: intentions.yaml says %s but the manifest pins %s", seed, cfg.Format, set.Manifest.Format)
	}
}

// Select returns the scenarios carrying any of classes (all when empty) and,
// when ids is non-empty, only those ids. An unknown id or class is an error.
func (set *Set) Select(classes, ids []string) ([]*Scenario, error) {
	for _, c := range classes {
		if _, ok := set.Manifest.Classes[c]; !ok {
			return nil, fmt.Errorf("--classes: unknown class %s", c)
		}
	}
	byID := map[string]bool{}
	for _, s := range set.Scenarios {
		byID[s.ID] = true
	}
	for _, id := range ids {
		if !byID[id] {
			return nil, fmt.Errorf("--ids: unknown scenario %s", id)
		}
	}
	var out []*Scenario
	for _, s := range set.Scenarios {
		if len(ids) > 0 && !slices.Contains(ids, s.ID) {
			continue
		}
		if len(classes) > 0 && !slices.ContainsFunc(classes, s.HasClass) {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no scenario matches the selection")
	}
	return out, nil
}
