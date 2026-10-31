package refcli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/refcli"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
)

func cli(t *testing.T) refcli.CLI {
	t.Helper()
	if _, err := exec.LookPath("intentions"); err != nil {
		t.Skip("intentions not on PATH")
	}
	return refcli.CLI{Path: "intentions"}
}

// pinned loads the shipped set and skips when the CLI on PATH is not the
// version it pins.
func pinned(t *testing.T) (refcli.CLI, *scenario.Set) {
	t.Helper()
	c := cli(t)
	set, err := scenario.Load("../../scenarios")
	if err != nil {
		t.Fatal(err)
	}
	if v, _, err := c.Version(); err != nil || v != set.Manifest.ReferenceCLI {
		t.Skipf("intentions on PATH is %s, the set pins %s", v, set.Manifest.ReferenceCLI)
	}
	return c, set
}

func TestShippedSetPassesCheck(t *testing.T) {
	c, set := pinned(t)
	if err := c.Check(set, set.Scenarios, t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

// copySet copies the shipped set so that a test can break it.
func copySet(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir("../../scenarios", func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("../../scenarios", p)
		if e.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestGoldServingDraftTerminusRefused(t *testing.T) {
	c, _ := pinned(t)
	root := copySet(t)
	// The budget intention serves a terminus the gold leaves tentative.
	gold := `- key: budget
  kind: intention
  duration: PT90M
  window: {calendar: 2026-W42}
  serves: [draft]
- key: draft
  kind: terminus
  stability: tentative
`
	if err := os.WriteFile(filepath.Join(root, "gold", "intention-budget-01.yaml"), []byte(gold), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := scenario.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	sel, _ := set.Select(nil, []string{"intention-budget-01"})
	err = c.Check(set, sel, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "intention-budget-01") || !strings.Contains(err.Error(), "key budget") || !strings.Contains(err.Error(), "unserved") {
		t.Fatalf("want refusal naming the scenario and the unserved key, got %v", err)
	}
}

func TestSeedWithUnservedIntentionRefused(t *testing.T) {
	c, _ := pinned(t)
	root := copySet(t)
	bad := `id: int_01a18f3d-0000-7000-8000-000000000001
subject: https://example.com/people/ada
title: Unserved
duration: PT30M
window: {calendar: 2026-W42}
stability: tentative
serves: []
source: {author: https://example.com/people/ada}
timestamp: 2026-10-05T00:00:00Z
acknowledgements: []
`
	if err := os.WriteFile(filepath.Join(root, "seeds", "one-firm-terminus", "intentions", "int_01a18f3d-0000-7000-8000-000000000001.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := scenario.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	err = c.Check(set, set.Scenarios, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "seed seeds/one-firm-terminus") || !strings.Contains(err.Error(), "unserved") {
		t.Fatalf("want refusal naming the seed and the validation error, got %v", err)
	}
}

func TestVersionPin(t *testing.T) {
	c, set := pinned(t)
	set.Manifest.ReferenceCLI = "0.0.1"
	if err := c.Check(set, set.Scenarios[:1], t.TempDir()); err == nil || !strings.Contains(err.Error(), "pins 0.0.1") {
		t.Fatalf("version mismatch accepted: %v", err)
	}
}

func TestBounds(t *testing.T) {
	c := cli(t)
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.FixedZone("AEDT", 11*3600))
	iv, err := c.Bounds("../../scenarios/seeds/one-firm-terminus", "2026-10-14", "14:00/15:30", now, now.AddDate(-1, 0, 0), now.AddDate(2, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(iv) != 1 || iv[0].End.Sub(iv[0].Start) != 90*time.Minute || iv[0].Start.UTC().Format(time.RFC3339) != "2026-10-14T03:00:00Z" {
		t.Errorf("intervals %+v", iv)
	}
}
