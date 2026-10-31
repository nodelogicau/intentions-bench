// Package prompts holds every prompt the harness sends. Each prompt's hash is
// recorded in the run directory and the report.
//
// Copied from particulars-bench (prompts/prompts.go) at commit
// 770728513dc4f5a84030b047e86903b70662dfc2, without its LongMemEval
// templates.
package prompts

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"strings"
)

//go:embed *.md
var files embed.FS

// Prompt names.
const (
	Phase      = "phase"
	Classifier = "classifier"
	Judge      = "judge"
)

// Names lists every prompt.
var Names = []string{Phase, Classifier, Judge}

// Get returns the named prompt (file name without .md) without its trailing
// newline. It panics on an unknown name: prompts are compiled in.
func Get(name string) string {
	b, err := files.ReadFile(name + ".md")
	if err != nil {
		panic(err)
	}
	return strings.TrimRight(string(b), "\n")
}

// Hash returns the SHA-256 of the named prompt as Get returns it.
func Hash(name string) string {
	h := sha256.Sum256([]byte(Get(name)))
	return hex.EncodeToString(h[:])
}

// Hashes returns every prompt's hash by name.
func Hashes() map[string]string {
	out := map[string]string{}
	for _, n := range Names {
		out[n] = Hash(n)
	}
	return out
}
