// Package refcli drives the pinned build of the reference CLI, `intentions`,
// for the checks the harness does not reimplement: validating seeds and
// gold workspaces, and computing the instants a window admits. The scored
// implementation is never consulted for either.
package refcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CLI is a reference CLI binary.
type CLI struct {
	// Path is the binary, found on PATH when it has no separator.
	Path string
}

func (c CLI) run(args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(c.Path, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// Version returns the binary's version and the format version it writes.
func (c CLI) Version() (version, format string, err error) {
	out, stderr, err := c.run("version", "--json")
	if err != nil {
		return "", "", fmt.Errorf("%s version: %w: %s", c.Path, err, strings.TrimSpace(string(stderr)))
	}
	var v struct {
		Version string `json:"version"`
		Format  string `json:"format"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", "", fmt.Errorf("%s version: %w", c.Path, err)
	}
	return v.Version, v.Format, nil
}

// Finding is one validation finding.
type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path,omitempty"`
	ID       string `json:"id,omitempty"`
	Message  string `json:"message"`
}

// Validate runs validate on a workspace and returns its findings. A
// workspace with errors is not an error here; the caller reads Errors.
func (c CLI) Validate(ws string) ([]Finding, error) {
	out, stderr, err := c.run("validate", "--workspace", ws, "--json")
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 4) {
		return nil, fmt.Errorf("validate %s: %w: %s", ws, err, strings.TrimSpace(string(stderr)))
	}
	var v struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("validate %s: %w", ws, err)
	}
	return v.Findings, nil
}

// Errors returns the findings of severity error.
func Errors(fs []Finding) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Severity == "error" {
			out = append(out, f)
		}
	}
	return out
}

// Index regenerates a workspace's index.
func (c CLI) Index(ws string) error {
	_, stderr, err := c.run("index", "--workspace", ws, "--json")
	if err != nil {
		return fmt.Errorf("index %s: %w: %s", ws, err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

// Interval is a half-open span of instants.
type Interval struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Bounds returns the intervals a calendar and clock anchor admit under the
// resolver context of the workspace ws, at now, with open bounds clamped to
// [from, to].
func (c CLI) Bounds(ws, calendar, clock string, now, from, to time.Time) ([]Interval, error) {
	args := []string{"bounds", "--workspace", ws, "--json",
		"--now", now.Format(time.RFC3339),
		"--horizon-start", from.Format(time.RFC3339), "--horizon-end", to.Format(time.RFC3339)}
	if calendar != "" {
		args = append(args, "--calendar", calendar)
	}
	if clock != "" {
		args = append(args, "--clock", clock)
	}
	out, stderr, err := c.run(args...)
	if err != nil {
		return nil, fmt.Errorf("bounds calendar=%q clock=%q: %w: %s", calendar, clock, err, strings.TrimSpace(string(stderr)))
	}
	var v struct {
		Intervals []Interval `json:"intervals"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, fmt.Errorf("bounds: %w", err)
	}
	return v.Intervals, nil
}
