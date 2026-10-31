package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/nodelogicau/intentions-bench/internal/episode"
	"github.com/nodelogicau/intentions-bench/internal/refcli"
	"github.com/nodelogicau/intentions-bench/internal/report"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/score"
	"github.com/nodelogicau/intentions-bench/prompts"
)

// printPlan prints what the run phase will do before its first model call.
func printPlan(w io.Writer, o *options, set *scenario.Set, sel []*scenario.Scenario) {
	fmt.Fprintf(w, "Plan (run)\n")
	fmt.Fprintf(w, "  scenario set: %s (format %s, reference CLI %s)\n", set.Root, set.Manifest.Format, set.Manifest.ReferenceCLI)
	byClass := map[string]int{}
	beats := 0
	for _, s := range sel {
		for _, c := range s.Class {
			byClass[c]++
		}
		beats += len(s.Beats)
	}
	fmt.Fprintf(w, "  %d scenarios, %d beats; by class:\n", len(sel), beats)
	var classes []string
	for c := range byClass {
		classes = append(classes, c)
	}
	sort.Strings(classes)
	for _, c := range classes {
		fmt.Fprintf(w, "    %-28s %3d\n", c, byClass[c])
	}
	for _, im := range o.impls {
		fmt.Fprintf(w, "  implementation %s: %s\n", im.Label, im.Command)
	}
	fmt.Fprintf(w, "  repeats %d; up to %d card answers between beats\n", o.repeats, o.maxExchanges)
	runs := len(sel) * len(o.impls) * o.repeats
	// Each beat and each card answer is one person message; each message
	// takes 1 to max-turns agent calls and at most one classifier call.
	msgsMin, msgsMax := beats, beats*(1+o.maxExchanges)
	scale := len(o.impls) * o.repeats
	fmt.Fprintf(w, "  agent %s (effort %s): %d to %d model calls over %d scenario runs\n",
		o.agent.Model, o.agent.Effort, msgsMin*scale, msgsMax*o.agent.MaxTurns*scale, runs)
	fmt.Fprintf(w, "  classifier %s (effort %s): up to %d model calls\n", o.classifier.Model, o.classifier.Effort, msgsMax*scale)
	fmt.Fprintf(w, "  judge %s (effort %s): called by score only when structure ties\n", o.judge.Model, o.judge.Effort)
}

func runRun(ctx context.Context, o *options, d Deps) error {
	if len(o.impls) == 0 {
		return fmt.Errorf("run needs at least one --impl")
	}
	set, err := scenario.Load(o.scenarios)
	if err != nil {
		return err
	}
	sel, err := set.Select(o.classes, o.ids)
	if err != nil {
		return err
	}
	cli := refcli.CLI{Path: o.referenceCLI}
	tmp, err := os.MkdirTemp("", "intentions-bench-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, lookErr := exec.LookPath(o.referenceCLI); lookErr != nil && o.dryRun {
		fmt.Fprintf(d.Stderr, "note: reference CLI %s not found; seed and gold checks skipped in this dry run\n", o.referenceCLI)
	} else if err := cli.Check(set, sel, filepath.Join(tmp, "gold")); err != nil {
		return err
	}
	printPlan(d.Stdout, o, set, sel)
	if o.dryRun {
		return nil
	}
	version, _, err := cli.Version()
	if err != nil {
		return err
	}

	path := o.runDir
	if path == "" {
		path = filepath.Join(o.resultsDir, d.Now().UTC().Format("20060102T150405Z"))
	}
	if _, ok, _ := (&run.Dir{Path: path}).Meta(); ok {
		return fmt.Errorf("%s already holds a run; give a new --run", path)
	}
	dir, err := run.Open(path)
	if err != nil {
		return err
	}
	meta := run.Meta{
		Created: d.Now().UTC(), HarnessCommit: harnessCommit(), Selection: run.Selection{Classes: o.classes, IDs: o.ids},
		Impls: o.impls, ReferenceCLI: version, Agent: o.agent, Classifier: o.classifier, Judge: o.judge,
		Prompts: prompts.Hashes(), Repeats: o.repeats, MaxExchanges: o.maxExchanges,
		ScenarioSet: run.ScenarioSet{Source: set.Root, Scenarios: map[string]string{}, Format: set.Manifest.Format, ReferenceCLI: set.Manifest.ReferenceCLI},
	}
	if meta.ScenarioSet.SHA256, err = set.Hash(); err != nil {
		return err
	}
	for _, s := range sel {
		meta.ScenarioIDs = append(meta.ScenarioIDs, s.ID)
		if meta.ScenarioSet.Scenarios[s.ID], err = s.Hash(set.Root); err != nil {
			return err
		}
	}
	if err := run.CopyDir(set.Root, dir.ScenariosDir()); err != nil {
		return err
	}
	if err := dir.WriteMeta(meta); err != nil {
		return err
	}

	key := d.Getenv(APIKeyVar)
	agentModel, err := d.NewProvider(o.agent, key)
	if err != nil {
		return err
	}
	classifierModel, err := d.NewProvider(o.classifier, key)
	if err != nil {
		return err
	}
	type job struct {
		label  string
		s      *scenario.Scenario
		repeat int
	}
	var jobs []job
	for i := range o.impls {
		for _, s := range sel {
			for r := 0; r < o.repeats; r++ {
				jobs = append(jobs, job{o.impls[i].Label, s, r})
			}
		}
	}
	byLabel := map[string]int{}
	for i, im := range o.impls {
		byLabel[im.Label] = i
	}
	var mu sync.Mutex
	var firstErr error
	failures := 0
	forEach(ctx, len(jobs), o.concurrency, func(i int) {
		j := jobs[i]
		rec, err := episode.Run(ctx, episode.Config{
			Impl: o.impls[byLabel[j.label]], Scenario: j.s, Repeat: j.repeat,
			SeedDir:   filepath.Join(dir.ScenariosDir(), j.s.Seed),
			Workspace: filepath.Join(dir.RunDir(j.label, j.s.ID, j.repeat), "workspace"),
			Agent:     o.agent, AgentModel: agentModel, Classifier: o.classifier, ClassifierModel: classifierModel,
			MaxExchanges: o.maxExchanges, Now: d.Now,
		})
		werr := dir.WriteRecord(rec)
		mu.Lock()
		defer mu.Unlock()
		for _, e := range []error{err, werr} {
			if e != nil && firstErr == nil && !errors.Is(e, context.Canceled) {
				firstErr = e
			}
		}
		status := fmt.Sprintf("%d calls, %d licences", len(rec.Calls), len(rec.Ledger))
		if rec.Failure != "" {
			failures++
			status = "FAILED (" + rec.FailureKind + "): " + firstLine(rec.Failure)
		}
		fmt.Fprintf(d.Stderr, "run %s %s#%d: %s\n", j.label, j.s.ID, j.repeat, status)
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if firstErr != nil {
		return firstErr
	}
	fmt.Fprintf(d.Stdout, "%d scenario runs (%d infrastructure failures) written to %s; next: intentions-bench score --run %s\n", len(jobs), failures, dir.Path, dir.Path)
	return nil
}

// loaded is a run directory and the scenario set fixed in it.
type loaded struct {
	dir       *run.Dir
	meta      run.Meta
	set       *scenario.Set
	scenarios []*scenario.Scenario
}

// load opens --run, or the latest run, and reads the scenario set copied
// into it when it started, so that later phases read only the run directory.
func load(o *options, d Deps) (*loaded, error) {
	path := o.runDir
	if path == "" {
		latest, err := run.Latest(o.resultsDir)
		if err != nil {
			return nil, err
		}
		path = latest
	}
	dir := &run.Dir{Path: path}
	meta, ok, err := dir.Meta()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s is not a run (no run.json)", path)
	}
	set, err := scenario.Load(dir.ScenariosDir())
	if err != nil {
		return nil, fmt.Errorf("the scenario set fixed in %s: %w", path, err)
	}
	ids, err := set.Select(nil, meta.ScenarioIDs)
	if err != nil {
		return nil, err
	}
	if len(o.impls) > 0 || len(o.classes) > 0 || len(o.ids) > 0 {
		fmt.Fprintf(d.Stderr, "note: --impl, --classes and --ids are ignored: run %s keeps what it started with\n", path)
	}
	fmt.Fprintf(d.Stderr, "using run %s (scenario set sha256 %s, %d scenarios)\n", path, meta.ScenarioSet.SHA256, len(ids))
	return &loaded{dir: dir, meta: meta, set: set, scenarios: ids}, nil
}

func runScore(ctx context.Context, o *options, d Deps) error {
	l, err := load(o, d)
	if err != nil {
		return err
	}
	cli := refcli.CLI{Path: o.referenceCLI}
	version, _, err := cli.Version()
	if err != nil {
		return err
	}
	if version != l.meta.ReferenceCLI {
		return fmt.Errorf("reference CLI %s is version %s; run %s was checked with %s, and windows must be bounded by the same build", o.referenceCLI, version, l.dir.Path, l.meta.ReferenceCLI)
	}
	sc := &score.Scorer{CLI: cli, JudgeSettings: l.meta.Judge}
	if key := d.Getenv(APIKeyVar); key != "" {
		if sc.Judge, err = d.NewProvider(l.meta.Judge, key); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(d.Stderr, "note: %s is not set; a run whose objects tie by structure cannot be judged and will fail scoring\n", APIKeyVar)
	}
	byID := map[string]*scenario.Scenario{}
	for _, s := range l.scenarios {
		byID[s.ID] = s
	}
	for _, im := range l.meta.Impls {
		recs, err := l.dir.Records(im.Label)
		if err != nil {
			return err
		}
		stream := "scores/" + im.Label
		if err := l.dir.Reset(stream); err != nil {
			return err
		}
		var mu sync.Mutex
		var firstErr error
		forEach(ctx, len(recs), o.concurrency, func(i int) {
			rec := recs[i]
			s := byID[rec.Scenario]
			if s == nil {
				return
			}
			final := filepath.Join(l.dir.RunDir(rec.Impl, rec.Scenario, rec.Repeat), "workspace")
			res, err := sc.Score(ctx, s, filepath.Join(l.dir.ScenariosDir(), s.Seed), final, rec)
			if err == nil {
				err = l.dir.Append(stream, res)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s %s#%d: %w", rec.Impl, rec.Scenario, rec.Repeat, err)
				}
				return
			}
			fmt.Fprintf(d.Stderr, "score %s %s#%d: %s\n", rec.Impl, rec.Scenario, rec.Repeat, summary(res))
		})
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if firstErr != nil {
			return firstErr
		}
	}
	fmt.Fprintf(d.Stdout, "scores written to run %s; next: intentions-bench report --run %s\n", l.dir.Path, l.dir.Path)
	return nil
}

func summary(r score.Result) string {
	if r.Failure != "" {
		return "not scored (infrastructure failure: " + r.FailureKind + ")"
	}
	var errs []string
	for name, bad := range map[string]bool{"rungs": r.Errors.Rungs, "fields": r.Errors.Fields, "restraint": r.Errors.Restraint} {
		if bad {
			errs = append(errs, name)
		}
	}
	if len(errs) == 0 {
		return "pass"
	}
	sort.Strings(errs)
	return "errors in " + strings.Join(errs, ", ")
}

func runReport(ctx context.Context, o *options, d Deps) error {
	l, err := load(o, d)
	if err != nil {
		return err
	}
	in := report.Input{
		Meta: l.meta, Scenarios: l.scenarios, Classes: l.set.Manifest.Classes,
		Records: map[string][]run.Record{}, Results: map[string][]score.Result{}, Now: d.Now(),
	}
	for _, im := range l.meta.Impls {
		if in.Records[im.Label], err = l.dir.Records(im.Label); err != nil {
			return err
		}
		if in.Results[im.Label], err = run.Read[score.Result](l.dir, "scores/"+im.Label); err != nil {
			return err
		}
		res := in.Results[im.Label]
		sort.Slice(res, func(i, j int) bool {
			if res[i].Scenario != res[j].Scenario {
				return res[i].Scenario < res[j].Scenario
			}
			return res[i].Repeat < res[j].Repeat
		})
		if len(res) == 0 {
			return fmt.Errorf("run %s has no scores for %s; run intentions-bench score first", l.dir.Path, im.Label)
		}
	}
	r := report.Build(in)
	jsonPath, mdPath := l.dir.File("report.json"), l.dir.File("report.md")
	if err := run.WriteJSON(jsonPath, r); err != nil {
		return err
	}
	md := report.Markdown(r)
	if err := os.WriteFile(mdPath, []byte(md), 0o644); err != nil {
		return err
	}
	fmt.Fprint(d.Stdout, md)
	fmt.Fprintf(d.Stderr, "wrote %s and %s\n", jsonPath, mdPath)
	return nil
}
