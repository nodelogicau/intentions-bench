// Package run is the directory a run's phases write to and read from, so that
// each phase can run on its own and later phases read nothing else.
//
// The directory shell (Dir, Latest, Reset, Append, Read) is copied from
// particulars-bench (internal/run) at commit
// 770728513dc4f5a84030b047e86903b70662dfc2; the records are this harness's
// own. Fixes made there must be ported by hand.
package run

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/impl"
)

// ScenarioSet identifies the scenario set a run fixed when it started.
type ScenarioSet struct {
	// Source is the directory the set was read from.
	Source string `json:"source"`
	// SHA256 is the content hash of the whole set as read from Source.
	SHA256 string `json:"sha256"`
	// Scenarios maps each selected scenario id to the hash of its file,
	// gold file, and seed directory.
	Scenarios map[string]string `json:"scenarios"`
	// Format and ReferenceCLI are what the set's manifest pins.
	Format       string `json:"format"`
	ReferenceCLI string `json:"reference_cli"`
}

// Selection is which scenarios a run took from the set.
type Selection struct {
	Classes []string `json:"classes,omitempty"`
	IDs     []string `json:"ids,omitempty"`
}

// Meta is fixed when a run starts: its scenarios, implementations, models,
// prompts and settings.
type Meta struct {
	Created       time.Time   `json:"created"`
	HarnessCommit string      `json:"harness_commit"`
	ScenarioSet   ScenarioSet `json:"scenario_set"`
	Selection     Selection   `json:"selection"`
	ScenarioIDs   []string    `json:"scenario_ids"`
	Impls         []impl.Impl `json:"impls"`
	// ReferenceCLI is the version of the reference CLI binary that checked
	// the set; scoring computes window bounds with the same build.
	ReferenceCLI string         `json:"reference_cli"`
	Agent        agent.Settings `json:"agent"`
	Classifier   agent.Settings `json:"classifier"`
	Judge        agent.Settings `json:"judge"`
	// Prompts maps each prompt's name to its SHA-256.
	Prompts      map[string]string `json:"prompt_sha256"`
	Repeats      int               `json:"repeats"`
	MaxExchanges int               `json:"max_exchanges"`
}

// Message is one message in a scenario run's transcript.
type Message struct {
	Exchange int `json:"exchange"`
	// Role is "person" or "agent".
	Role string `json:"role"`
	Text string `json:"text"`
	// Kind says where a person's message came from: "beat", "entry",
	// or "default". Entry names the card entry.
	Kind  string `json:"kind,omitempty"`
	Entry string `json:"entry,omitempty"`
	Beat  int    `json:"beat,omitempty"`
	// Capped marks an agent message whose loop hit the turn cap.
	Capped bool `json:"capped,omitempty"`
}

// Classification is one classifier call: what it saw and what it said.
type Classification struct {
	// Exchange is the exchange whose agent text was classified.
	Exchange int    `json:"exchange"`
	Input    string `json:"input"`
	Raw      string `json:"raw"`
	// Choice is an entry id, "default", or "none".
	Choice string `json:"choice"`
	// Malformed marks a reply that was none of those, taken as default.
	Malformed bool        `json:"malformed,omitempty"`
	Usage     agent.Usage `json:"usage"`
}

// Licence is an act the person licensed, and the exchange at which it
// entered the ledger.
type Licence struct {
	Act      string `json:"act"`
	Key      string `json:"key"`
	Exchange int    `json:"exchange"`
	// From is "beat:<n>" or "entry:<id>".
	From string `json:"from"`
}

// Call is one tool call in the call log.
type Call struct {
	Exchange int             `json:"exchange"`
	Tool     string          `json:"tool"`
	Args     json.RawMessage `json:"args"`
	ReadOnly bool            `json:"read_only"`
	Result   string          `json:"result"`
	IsError  bool            `json:"is_error,omitempty"`
	Code     string          `json:"code,omitempty"`
	Refused  bool            `json:"refused,omitempty"`
}

// ToolInfo is a tool the implementation listed.
type ToolInfo struct {
	Name      string `json:"name"`
	ReadOnly  bool   `json:"read_only"`
	Extension bool   `json:"extension,omitempty"`
}

// Record is one run of one scenario by one implementation.
type Record struct {
	Impl       string          `json:"impl"`
	Scenario   string          `json:"scenario"`
	Repeat     int             `json:"repeat"`
	ServerInfo impl.ServerInfo `json:"server_info"`
	Tools      []ToolInfo      `json:"tools,omitempty"`
	// InstructionsSHA256 hashes the initialize instructions the agent saw.
	InstructionsSHA256 string           `json:"instructions_sha256,omitempty"`
	Transcript         []Message        `json:"transcript"`
	Classifications    []Classification `json:"classifications"`
	Ledger             []Licence        `json:"ledger"`
	Calls              []Call           `json:"calls"`
	AgentUsage         agent.Usage      `json:"agent_usage"`
	ClassifierUsage    agent.Usage      `json:"classifier_usage"`
	Started            time.Time        `json:"started"`
	Wall               Duration         `json:"wall"`
	// Failure is an infrastructure failure: the run is excluded from every
	// score and counted separately. FailureKind is "start", "clock", or
	// "error".
	Failure     string `json:"failure,omitempty"`
	FailureKind string `json:"failure_kind,omitempty"`
	Stderr      string `json:"stderr,omitempty"`
}

// Duration marshals as a Go duration string.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	*d = Duration(v)
	return err
}

// Dir is a run directory.
type Dir struct {
	Path string
	mu   sync.Mutex
}

// Open returns the run directory at path, creating it.
func Open(path string) (*Dir, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	return &Dir{Path: path}, nil
}

// Latest returns the most recently named run under root.
func Latest(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("no runs under %s: %w", root, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(root, e.Name(), "run.json")); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("no runs under %s", root)
	}
	sort.Strings(names)
	return filepath.Join(root, names[len(names)-1]), nil
}

// File returns the path of name inside the run.
func (d *Dir) File(name string) string { return filepath.Join(d.Path, name) }

// ScenariosDir is the copy of the scenario set fixed when the run started.
func (d *Dir) ScenariosDir() string { return d.File("scenarios") }

// RunDir is the directory of one scenario run.
func (d *Dir) RunDir(implLabel, scenario string, repeat int) string {
	return d.File(filepath.Join("runs", implLabel, scenario, fmt.Sprint(repeat)))
}

// Meta reads run.json; ok is false when the run has not started.
func (d *Dir) Meta() (m Meta, ok bool, err error) {
	ok, err = ReadJSON(d.File("run.json"), &m)
	return m, ok, err
}

// WriteMeta writes run.json.
func (d *Dir) WriteMeta(m Meta) error { return WriteJSON(d.File("run.json"), m) }

// WriteRecord writes a scenario run's record.
func (d *Dir) WriteRecord(r Record) error {
	return WriteJSON(filepath.Join(d.RunDir(r.Impl, r.Scenario, r.Repeat), "record.json"), r)
}

// Records reads every scenario run recorded for an implementation, ordered
// by scenario and repeat.
func (d *Dir) Records(implLabel string) ([]Record, error) {
	paths, err := filepath.Glob(d.File(filepath.Join("runs", implLabel, "*", "*", "record.json")))
	if err != nil {
		return nil, err
	}
	var out []Record
	for _, p := range paths {
		var r Record
		if _, err := ReadJSON(p, &r); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Scenario != out[j].Scenario {
			return out[i].Scenario < out[j].Scenario
		}
		return out[i].Repeat < out[j].Repeat
	})
	return out, nil
}

// Reset empties a JSONL stream before a phase rewrites it.
func (d *Dir) Reset(stream string) error {
	p := d.File(stream + ".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, nil, 0o644)
}

// Append adds a record to a JSONL stream.
func (d *Dir) Append(stream string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.File(stream + ".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// Read decodes every record of a stream into a slice of T.
func Read[T any](d *Dir, stream string) ([]T, error) {
	f, err := os.Open(d.File(stream + ".jsonl"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, fmt.Errorf("%s: %w", stream, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

// ReadJSON decodes a JSON file; ok is false when it does not exist.
func ReadJSON(path string, v any) (bool, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(b, v)
}

// WriteJSON writes v as indented JSON, creating the directory.
func WriteJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// CopyDir copies the regular files under src to dst.
func CopyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if e.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !e.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
