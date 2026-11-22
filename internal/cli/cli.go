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

	"github.com/nodelogicau/agentloop/agent"
	"github.com/nodelogicau/intentions-bench/internal/impl"
)

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

run refuses to start without the API key of each provider it calls
(ANTHROPIC_API_KEY for anthropic, OPENAI_API_KEY for openai, or the variable
--api-key-env / --classifier-api-key-env names), except with --dry-run, which
prints the plan and exits without calling a model. score calls the judge only
when objects tie by structure, keyed by --judge-api-key-env or by the judge's
provider as the run recorded it.

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
	if phase == "run" && !o.dryRun {
		for _, r := range o.runRoles() {
			if d.Getenv(r.keyEnv) == "" {
				fmt.Fprintf(d.Stderr, "intentions-bench run: %s is not set; refusing to start a phase that calls a model (use --dry-run to see the plan)\n", r.keyEnv)
				return 1
			}
		}
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
	// The variables holding each role's API key.
	agentKeyEnv, classifierKeyEnv, judgeKeyEnv string
	// set names the flags given on the command line.
	set map[string]bool
}

// runRoles are the model roles the run phase calls.
func (o *options) runRoles() []role {
	return []role{{"agent", o.agent, o.agentKeyEnv}, {"classifier", o.classifier, o.classifierKeyEnv}}
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
	var agentF, classifierF, judgeF roleFlags
	providers := strings.Join(agent.Providers, ", ")
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
	for _, r := range []struct {
		prefix, what string
		f            *roleFlags
		s            *agent.Settings
		keyEnv       *string
	}{
		{"", "the agent", &agentF, &o.agent, &o.agentKeyEnv},
		{"classifier-", "the classifier", &classifierF, &o.classifier, &o.classifierKeyEnv},
		{"judge-", "the judge", &judgeF, &o.judge, &o.judgeKeyEnv},
	} {
		fs.StringVar(&r.f.provider, r.prefix+"provider", agent.ProviderAnthropic, "provider for "+r.what+": "+providers)
		fs.StringVar(&r.s.BaseURL, r.prefix+"base-url", "", "API base URL for "+r.what+" (openai default: "+agent.DefaultOpenAIBaseURL+")")
		fs.StringVar(r.keyEnv, r.prefix+"api-key-env", "", "`variable` holding the API key for "+r.what+" (default: ANTHROPIC_API_KEY or OPENAI_API_KEY by provider)")
		fs.StringVar(&r.f.extraBody, r.prefix+"extra-body", "", "JSON object merged into every request body for "+r.what)
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	o.classifier.MaxTokens, o.classifier.MaxTurns = 4000, 1
	o.judge.MaxTokens, o.judge.MaxTurns = 4000, 1
	o.set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { o.set[f.Name] = true })
	if err := configureRole("", agentF, &o.agent, &o.agentKeyEnv, o.set); err != nil {
		return nil, err
	}
	if err := configureRole("classifier-", classifierF, &o.classifier, &o.classifierKeyEnv, o.set); err != nil {
		return nil, err
	}
	if err := configureRole("judge-", judgeF, &o.judge, &o.judgeKeyEnv, o.set); err != nil {
		return nil, err
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

// agentloopVersion is the agentloop module version built in, so that a
// change of agent loop shows in the run's metadata.
func agentloopVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == "github.com/nodelogicau/agentloop" {
				if d.Replace != nil {
					return d.Version + " => " + d.Replace.Path + " " + d.Replace.Version
				}
				return d.Version
			}
		}
	}
	return "unknown"
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
