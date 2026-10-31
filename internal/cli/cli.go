// Package cli is the intentions-bench command: phases run, score, and report,
// each separately runnable against one run directory.
//
// The command shell (Deps, flag handling, the key refusal, run directory
// resolution, forEach) is copied from particulars-bench (internal/cli) at
// commit 770728513dc4f5a84030b047e86903b70662dfc2. Fixes made there must be
// ported by hand.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/impl"
)

// APIKeyVar holds the Anthropic API key.
const APIKeyVar = "ANTHROPIC_API_KEY"

var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// Deps are the command's connections to the outside world.
type Deps struct {
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	// NewProvider returns a model provider; it is called only after the
	// plan is printed and only when a phase will call a model.
	NewProvider func(s agent.Settings, apiKey string) (agent.Provider, error)
	Now         func() time.Time
}

// DefaultDeps uses the process environment and the Anthropic API.
func DefaultDeps() Deps {
	return Deps{Stdout: os.Stdout, Stderr: os.Stderr, Getenv: os.Getenv, NewProvider: agent.New, Now: time.Now}
}

const usage = `intentions-bench measures Intentions Format implementations, with the agent
that uses them, on scripted scenarios.

Usage:
  intentions-bench <phase> [flags]

Phases:
  run      check the scenario set with the pinned reference CLI, print the plan,
           and run every selected scenario against every --impl into a new run
           directory
  score    match each run's objects to gold, and score rungs, fields and
           restraint; the judge model is called only when structure ties
  report   write report.json (intentions-bench-report/1) and report.md

run refuses to start without ANTHROPIC_API_KEY, except with --dry-run, which
prints the plan and exits without calling a model.

Run 'intentions-bench <phase> -h' for flags.
`

// Run executes the command and returns its exit code.
func Run(ctx context.Context, args []string, d Deps) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(d.Stdout, usage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	// Phases report progress from concurrent workers.
	d.Stderr = &lockedWriter{w: d.Stderr}
	phase := args[0]
	var fn func(context.Context, *options, Deps) error
	switch phase {
	case "run":
		fn = runRun
	case "score":
		fn = runScore
	case "report":
		fn = runReport
	default:
		fmt.Fprintf(d.Stderr, "unknown phase %q\n\n%s", phase, usage)
		return 2
	}
	o, err := parseFlags(phase, args[1:], d.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(d.Stderr, "intentions-bench %s: %v\n", phase, err)
		return 2
	}
	if phase == "run" && !o.dryRun && d.Getenv(APIKeyVar) == "" {
		fmt.Fprintf(d.Stderr, "intentions-bench run: %s is not set; refusing to start a phase that calls a model (use --dry-run to see the plan)\n", APIKeyVar)
		return 1
	}
	if err := fn(ctx, o, d); err != nil {
		fmt.Fprintf(d.Stderr, "intentions-bench %s: %v\n", phase, err)
		return 1
	}
	return 0
}

type options struct {
	phase        string
	impls        []impl.Impl
	scenarios    string
	classes      []string
	ids          []string
	repeats      int
	maxExchanges int
	concurrency  int
	dryRun       bool
	runDir       string
	resultsDir   string
	referenceCLI string
	agent        agent.Settings
	classifier   agent.Settings
	judge        agent.Settings
}

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }
func (l *listFlag) Set(s string) error {
	*l = append(*l, s)
	return nil
}

var labelRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func parseFlags(phase string, args []string, stderr io.Writer) (*options, error) {
	fs := flag.NewFlagSet("intentions-bench "+phase, flag.ContinueOnError)
	fs.SetOutput(stderr)
	o := &options{phase: phase}
	var impls listFlag
	var classes, ids string
	fs.Var(&impls, "impl", "implementation under test, `label='command … {workspace} …'` (repeatable)")
	fs.StringVar(&o.scenarios, "scenarios", "scenarios", "scenario set directory (run phase)")
	fs.StringVar(&classes, "classes", "", "comma-separated classes; run only scenarios carrying one of them")
	fs.StringVar(&ids, "ids", "", "comma-separated scenario ids to run")
	fs.IntVar(&o.repeats, "repeats", 1, "runs of each scenario per implementation, for pass^k")
	fs.IntVar(&o.maxExchanges, "max-exchanges", 4, "card answers sent between two beats before the next beat")
	fs.IntVar(&o.concurrency, "concurrency", 4, "scenario runs at once")
	fs.BoolVar(&o.dryRun, "dry-run", false, "check the scenario set, print the plan, and exit without calling a model")
	fs.StringVar(&o.runDir, "run", "", "run directory (default: a new one under --results for run; the latest for score and report)")
	fs.StringVar(&o.resultsDir, "results", "results", "directory holding run directories")
	fs.StringVar(&o.referenceCLI, "reference-cli", "intentions", "the pinned reference CLI binary, for validate and bounds")
	fs.StringVar(&o.agent.Model, "model", "claude-opus-5-5", "agent model")
	fs.StringVar(&o.agent.Effort, "effort", "medium", "agent effort")
	fs.Int64Var(&o.agent.MaxTokens, "max-tokens", 16000, "agent max_tokens per model call")
	fs.IntVar(&o.agent.MaxTurns, "max-turns", 20, "agent model calls per person message")
	fs.StringVar(&o.classifier.Model, "classifier-model", "claude-opus-5-5", "classifier model")
	fs.StringVar(&o.classifier.Effort, "classifier-effort", "low", "classifier effort")
	fs.StringVar(&o.judge.Model, "judge-model", "claude-opus-5-5", "judge model (score phase, ties only)")
	fs.StringVar(&o.judge.Effort, "judge-effort", "low", "judge effort")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	o.classifier.MaxTokens, o.classifier.MaxTurns = 4000, 1
	o.judge.MaxTokens, o.judge.MaxTurns = 4000, 1
	for name, e := range map[string]string{"effort": o.agent.Effort, "classifier-effort": o.classifier.Effort, "judge-effort": o.judge.Effort} {
		if e != "" && !contains(efforts, e) {
			return nil, fmt.Errorf("--%s %q: use one of %v", name, e, efforts)
		}
	}
	seen := map[string]bool{}
	for _, s := range impls {
		im, err := impl.Parse(s)
		if err != nil {
			return nil, err
		}
		if !labelRE.MatchString(im.Label) {
			return nil, fmt.Errorf("--impl label %q: use letters, digits, '.', '_', '-'", im.Label)
		}
		if seen[im.Label] {
			return nil, fmt.Errorf("--impl label %q given twice", im.Label)
		}
		seen[im.Label] = true
		o.impls = append(o.impls, im)
	}
	o.classes, o.ids = split(classes), split(ids)
	if o.concurrency < 1 || o.repeats < 1 || o.agent.MaxTurns < 1 || o.maxExchanges < 0 {
		return nil, fmt.Errorf("--concurrency, --repeats and --max-turns must be at least 1, and --max-exchanges at least 0")
	}
	return o, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func harnessCommit() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		var rev, modified string
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				modified = s.Value
			}
		}
		if rev != "" {
			if modified == "true" {
				rev += "-dirty"
			}
			return rev
		}
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	rev := strings.TrimSpace(string(out))
	if err := exec.Command("git", "diff", "--quiet", "HEAD").Run(); err != nil {
		rev += "-dirty"
	}
	return rev
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// forEach runs f for i in [0, n) with at most conc at once.
func forEach(ctx context.Context, n, conc int, f func(i int)) {
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			f(i)
		}(i)
	}
	wg.Wait()
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}
