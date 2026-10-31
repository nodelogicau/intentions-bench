// Package episode runs one scenario against one implementation: it starts
// the implementation on a copy of the seed, sends the beats in order, routes
// the agent's questions to the card through the classifier, keeps the licence
// ledger and the call log, and checks the implementation's clock afterwards.
package episode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/impl"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/workspace"
	"github.com/nodelogicau/intentions-bench/prompts"
)

// Classifier choices other than an entry id.
const (
	ChoiceDefault = "default"
	ChoiceNone    = "none"
)

// Config is one scenario run.
type Config struct {
	Impl     impl.Impl
	Scenario *scenario.Scenario
	Repeat   int
	// SeedDir is the scenario's seed; Workspace is the fresh directory the
	// implementation runs in, which holds the final workspace afterwards.
	SeedDir   string
	Workspace string

	Agent           agent.Settings
	AgentModel      agent.Provider
	Classifier      agent.Settings
	ClassifierModel agent.Provider
	MaxExchanges    int
	// Now returns the wall clock, for the run's duration.
	Now func() time.Time
}

// PhasePrompt is the phase prompt for a scenario: the date and that the agent
// helps one person, and nothing of the format's rules.
func PhasePrompt(now time.Time) string {
	return strings.ReplaceAll(prompts.Get(prompts.Phase), "{date}", now.Format("Monday 2 January 2006, 15:04 (UTC-07:00)"))
}

// ClassifierInput is what the classifier sees: the card's ids and when
// lines, and the agent's final text.
func ClassifierInput(card []scenario.Entry, text string) string {
	var entries []string
	for _, e := range card {
		entries = append(entries, fmt.Sprintf("- %s: %s", e.ID, e.When))
	}
	if len(entries) == 0 {
		entries = append(entries, "(none)")
	}
	return strings.NewReplacer("{entries}", strings.Join(entries, "\n"), "{text}", text).Replace(prompts.Get(prompts.Classifier))
}

// ParseChoice reads a classifier reply: an entry id, default, or none, with
// surrounding quotes, backticks and punctuation ignored. Anything else is
// malformed and taken as default.
func ParseChoice(card []scenario.Entry, raw string) (choice string, malformed bool) {
	s := strings.TrimSpace(raw)
	s, _, _ = strings.Cut(s, "\n")
	s = strings.Trim(strings.TrimSpace(s), "`'\".:*")
	for _, e := range card {
		if strings.EqualFold(s, e.ID) {
			return e.ID, false
		}
	}
	switch strings.ToLower(s) {
	case ChoiceDefault:
		return ChoiceDefault, false
	case ChoiceNone:
		return ChoiceNone, false
	}
	return ChoiceDefault, true
}

// Run runs the scenario and returns its record. An infrastructure failure
// is recorded in the record, not returned; the error is for a context
// cancellation or a failure to prepare the workspace.
func Run(ctx context.Context, c Config) (rec run.Record, err error) {
	s := c.Scenario
	rec = run.Record{
		Impl: c.Impl.Label, Scenario: s.ID, Repeat: c.Repeat, Started: c.Now().UTC(),
		Transcript: []run.Message{}, Classifications: []run.Classification{}, Ledger: []run.Licence{}, Calls: []run.Call{},
	}
	start := c.Now()
	defer func() { rec.Wall = run.Duration(c.Now().Sub(start)) }()
	if err = os.RemoveAll(c.Workspace); err != nil {
		return rec, err
	}
	if err = run.CopyDir(c.SeedDir, c.Workspace); err != nil {
		return rec, err
	}
	if err := os.Remove(filepath.Join(c.Workspace, "keys.yaml")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return rec, err
	}

	srv, err := impl.Start(ctx, c.Impl, c.Workspace, s.Now)
	if err != nil {
		rec.Failure, rec.FailureKind = err.Error(), "start"
		var se *impl.StartError
		if errors.As(err, &se) {
			rec.Stderr = se.Stderr
		}
		return rec, ctx.Err()
	}
	rec.ServerInfo = srv.Info
	if srv.Instructions != "" {
		h := sha256.Sum256([]byte(srv.Instructions))
		rec.InstructionsSHA256 = hex.EncodeToString(h[:])
	}
	readOnly := map[string]bool{}
	for _, t := range srv.Tools {
		readOnly[t.Name] = impl.ReadOnly(t)
		rec.Tools = append(rec.Tools, run.ToolInfo{Name: t.Name, ReadOnly: impl.ReadOnly(t), Extension: !impl.IsReferenceTool(t.Name)})
	}
	tools, err := impl.ToolDefs(srv.Tools)
	if err != nil {
		_ = srv.Close()
		rec.Failure, rec.FailureKind = err.Error(), "start"
		return rec, ctx.Err()
	}

	log := &recorder{srv: srv, readOnly: readOnly}
	session := agent.NewSession(c.AgentModel, log, agent.Request{
		Settings: c.Agent, Instructions: srv.Instructions, Phase: PhasePrompt(s.Now), Tools: tools,
	})
	err = converse(ctx, c, &rec, session, log)
	rec.Calls = log.calls
	rec.Stderr = srv.Stderr()
	_ = srv.Close()
	if err != nil {
		rec.Failure, rec.FailureKind = err.Error(), "error"
		return rec, ctx.Err()
	}
	wall := c.Now().Sub(start)
	if msg, err := Canary(c.SeedDir, c.Workspace, s.Now, wall); err != nil {
		rec.Failure, rec.FailureKind = err.Error(), "error"
	} else if msg != "" {
		rec.Failure, rec.FailureKind = msg, "clock"
	}
	return rec, ctx.Err()
}

// converse sends the beats and the card's answers, recording the transcript,
// the classifications and the ledger.
func converse(ctx context.Context, c Config, rec *run.Record, session *agent.Session, log *recorder) error {
	s := c.Scenario
	exchange := 0
	licensed := map[scenario.Licence]bool{}
	grant := func(from string, ls []scenario.Licence) {
		for _, l := range ls {
			if licensed[l] {
				continue
			}
			licensed[l] = true
			rec.Ledger = append(rec.Ledger, run.Licence{Act: l.Act, Key: l.Key, Exchange: exchange, From: from})
		}
	}
	for bi, beat := range s.Beats {
		exchange++
		text := beat.Say
		rec.Transcript = append(rec.Transcript, run.Message{Exchange: exchange, Role: "person", Text: text, Kind: "beat", Beat: bi})
		grant(fmt.Sprintf("beat:%d", bi), beat.Licenses)
		for replies := 0; ; replies++ {
			log.setExchange(exchange)
			res, err := session.Send(ctx, text)
			rec.AgentUsage.Add(res.Usage)
			rec.Transcript = append(rec.Transcript, run.Message{Exchange: exchange, Role: "agent", Text: res.Text, Capped: res.Capped})
			if err != nil {
				return fmt.Errorf("agent at exchange %d: %w", exchange, err)
			}
			if res.Text == "" || replies >= c.MaxExchanges {
				break
			}
			input := ClassifierInput(s.Card, res.Text)
			raw, usage, err := agent.Complete(ctx, c.ClassifierModel, c.Classifier, "", input)
			rec.ClassifierUsage.Add(usage)
			if err != nil {
				return fmt.Errorf("classifier at exchange %d: %w", exchange, err)
			}
			choice, malformed := ParseChoice(s.Card, raw)
			rec.Classifications = append(rec.Classifications, run.Classification{
				Exchange: exchange, Input: input, Raw: raw, Choice: choice, Malformed: malformed, Usage: usage,
			})
			if choice == ChoiceNone {
				break
			}
			exchange++
			msg := run.Message{Exchange: exchange, Role: "person", Kind: choice}
			if e, ok := s.Entry(choice); ok {
				text = e.Say
				msg.Kind, msg.Entry = "entry", e.ID
				grant("entry:"+e.ID, e.Licenses)
			} else {
				text = s.Default
			}
			msg.Text = text
			rec.Transcript = append(rec.Transcript, msg)
		}
	}
	return nil
}

// recorder forwards tool calls to the implementation and logs each one with
// the exchange it was made in.
type recorder struct {
	srv      *impl.Server
	readOnly map[string]bool
	mu       sync.Mutex
	exchange int
	calls    []run.Call
}

func (r *recorder) setExchange(n int) {
	r.mu.Lock()
	r.exchange = n
	r.mu.Unlock()
}

func (r *recorder) Call(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	res, err := r.srv.CallTool(ctx, name, args)
	r.mu.Lock()
	defer r.mu.Unlock()
	call := run.Call{Exchange: r.exchange, Tool: name, Args: args, ReadOnly: r.readOnly[name],
		Result: res.Text, IsError: res.IsError, Code: res.Code, Refused: res.IsError && res.Code == "refused"}
	if err != nil {
		call.IsError, call.Result = true, "protocol error: "+err.Error()
	}
	r.calls = append(r.calls, call)
	return res.Text, res.IsError, err
}

// Canary compares the timestamp of every object the run wrote with the
// scenario's now. A difference greater than the run's wall-clock duration
// plus one minute means the implementation ignored INTENTIONS_NOW; the
// returned message names its clock as the cause.
func Canary(seedDir, finalDir string, now time.Time, wall time.Duration) (string, error) {
	before, err := workspace.Load(seedDir)
	if err != nil {
		return "", err
	}
	after, err := workspace.Load(finalDir)
	if err != nil {
		return "", err
	}
	created, changed := workspace.Diff(before, after)
	limit := wall + time.Minute
	for _, id := range append(created, changed...) {
		o := after.Objects[id]
		ts, ok := o.Timestamp()
		if !ok {
			continue
		}
		if d := ts.Sub(now); d > limit || -d > limit {
			return fmt.Sprintf("clock: %s carries timestamp %s but the scenario's now is %s; the implementation's clock ignores INTENTIONS_NOW",
				o.Path, ts.Format(time.RFC3339), now.Format(time.RFC3339)), nil
		}
	}
	return "", nil
}
