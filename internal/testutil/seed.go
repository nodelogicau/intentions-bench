package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// TerminusID is the firm terminus in the seed Seed writes, under key
// follow-through.
const TerminusID = "int_01a18f3d-f5a1-79c4-b1e2-f3e676ba45a0"

// Seed writes a minimal seed workspace: intentions.yaml, one firm terminus
// written by the person, and keys.yaml.
func Seed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"intentions.yaml": `format: intentions/0.2
hash: sha256
resolver: {timezone: Australia/Melbourne, hemisphere: south, horizon: P4W}
availability: {default_horizon: P13W}
defaults:
  subject: https://example.com/people/ada
  source: {author: https://example.com/people/ada}
`,
		"intentions/" + TerminusID + ".yaml": `id: ` + TerminusID + `
subject: https://example.com/people/ada
title: Being someone who follows through
stability: firm
serves: []
source: {author: https://example.com/people/ada}
timestamp: 2026-10-04T22:00:00Z
acknowledgements: []
`,
		"keys.yaml": "follow-through: " + TerminusID + "\n",
	}
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}
