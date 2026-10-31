package scenario_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nodelogicau/intentions-bench/internal/scenario"
)

const manifest = `format: intentions/0.2
reference_cli: 0.15.0
classes:
  rung/desire: "A passing remark held as a desire."
  restraint/select: "Selection waits for the person."
`

const seedConfig = `format: intentions/0.2
hash: sha256
resolver: {timezone: Australia/Melbourne, horizon: P4W}
availability: {default_horizon: P13W}
defaults:
  subject: https://example.com/people/ada
  source: {author: https://example.com/people/ada}
`

const good = `id: s1
class: [rung/desire, restraint/select]
now: 2026-10-12T09:00:00+11:00
seed: seeds/s
beats:
  - say: "I keep meaning to get back to running."
card:
  - id: pick
    when: offered candidate placements
    say: "The second one."
    licenses: [{act: select, key: running}]
default: "I'm not sure yet."
gold: gold/s1.yaml
`

const goldFile = `- key: running
  kind: desire
  title: Get back to running
`

// writeSet writes a scenario set with one scenario whose text is s.
func writeSet(t *testing.T, s string, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{
		"manifest.yaml":           manifest,
		"seeds/s/intentions.yaml": seedConfig,
		"seeds/s/keys.yaml":       "follow-through: int_x\n",
		"gold/s1.yaml":            goldFile,
		"scenarios/s1.yaml":       s,
	}
	for k, v := range files {
		all[k] = v
	}
	for name, body := range all {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func refused(t *testing.T, root string, want ...string) {
	t.Helper()
	_, err := scenario.Load(root)
	if err == nil {
		t.Fatalf("set accepted; want refusal naming %v", want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("refusal does not name %q:\n%v", w, err)
		}
	}
}

func TestLoadGood(t *testing.T) {
	set, err := scenario.Load(writeSet(t, good, nil))
	if err != nil {
		t.Fatal(err)
	}
	s := set.Scenarios[0]
	if s.ID != "s1" || s.Now.Format("2006-01-02T15:04") != "2026-10-12T09:00" || len(s.Gold) != 1 || s.Keys["follow-through"] != "int_x" {
		t.Errorf("parsed %+v", s)
	}
	if e, ok := s.Entry("pick"); !ok || e.Licenses[0] != (scenario.Licence{Act: "select", Key: "running"}) {
		t.Errorf("card %+v", s.Card)
	}
}

func TestMissingNow(t *testing.T) {
	refused(t, writeSet(t, strings.Replace(good, "now: 2026-10-12T09:00:00+11:00\n", "", 1), nil), "scenarios/s1.yaml", "missing now")
}

func TestNowWithoutOffset(t *testing.T) {
	refused(t, writeSet(t, strings.Replace(good, "+11:00", "", 1), nil), "scenarios/s1.yaml", "now")
}

func TestDoesNotParse(t *testing.T) {
	refused(t, writeSet(t, good+"beats: [\n", nil), "scenarios/s1.yaml", "does not parse")
}

func TestUnknownField(t *testing.T) {
	refused(t, writeSet(t, good+"colour: blue\n", nil), "scenarios/s1.yaml", "colour")
}

func TestDuplicateEntryID(t *testing.T) {
	dup := strings.Replace(good, "default:", `  - id: pick
    when: asked anything
    say: "Yes."
default:`, 1)
	refused(t, writeSet(t, dup, nil), "s1", "id pick")
}

func TestUnknownKey(t *testing.T) {
	refused(t, writeSet(t, strings.Replace(good, "key: running", "key: swimming", 1), nil), "s1", "swimming")
}

func TestFirmLicence(t *testing.T) {
	refused(t, writeSet(t, strings.Replace(good, "act: select", "act: intention_firm", 1), nil),
		"card entry pick", "intention_firm", "assent to firm versus assent to select")
}

func TestUnknownClass(t *testing.T) {
	refused(t, writeSet(t, strings.Replace(good, "rung/desire", "rung/wish", 1), nil), "class rung/wish")
}

func TestFormatVersionMismatch(t *testing.T) {
	root := writeSet(t, good, map[string]string{
		"seeds/s/intentions.yaml": strings.Replace(seedConfig, "intentions/0.2", "intentions/0.1", 1),
	})
	refused(t, root, "seed seeds/s", "intentions/0.1", "intentions/0.2")
}

func TestGoldUnknownField(t *testing.T) {
	refused(t, writeSet(t, good, map[string]string{"gold/s1.yaml": goldFile + "  colour: blue\n"}), "gold/s1.yaml", "colour")
}

func TestGoldKeepsDatesAsText(t *testing.T) {
	set, err := scenario.Load(writeSet(t, good, map[string]string{
		"gold/s1.yaml": goldFile + "- key: run\n  kind: intention\n  window: {calendar: 2026-10-14}\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	g, _ := set.Scenarios[0].GoldKey("run")
	if w := g.Fields["window"].(map[string]any); w["calendar"] != "2026-10-14" {
		t.Errorf("calendar %#v", w["calendar"])
	}
}

func TestSelect(t *testing.T) {
	set, err := scenario.Load(writeSet(t, good, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := set.Select([]string{"restraint/select"}, nil); err != nil || len(got) != 1 {
		t.Errorf("by class: %v %v", got, err)
	}
	if _, err := set.Select([]string{"rung/none"}, nil); err == nil {
		t.Error("unknown class accepted")
	}
	if _, err := set.Select(nil, []string{"s2"}); err == nil {
		t.Error("unknown id accepted")
	}
}

// TestShippedSet loads the repository's own scenario set.
func TestShippedSet(t *testing.T) {
	set, err := scenario.Load("../../scenarios")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, s := range set.Scenarios {
		for _, c := range s.Class {
			counts[c]++
		}
	}
	for c := range set.Manifest.Classes {
		if counts[c] < 3 {
			t.Errorf("class %s has %d scenarios, want at least 3", c, counts[c])
		}
	}
}
