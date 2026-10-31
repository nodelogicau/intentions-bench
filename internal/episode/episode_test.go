package episode_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/nodelogicau/intentions-bench/internal/agent"
	"github.com/nodelogicau/intentions-bench/internal/episode"
	"github.com/nodelogicau/intentions-bench/internal/fakeserver"
	"github.com/nodelogicau/intentions-bench/internal/impl"
	"github.com/nodelogicau/intentions-bench/internal/run"
	"github.com/nodelogicau/intentions-bench/internal/scenario"
	"github.com/nodelogicau/intentions-bench/internal/testutil"
)

func TestMain(m *testing.M) {
	fakeserver.Main()
	os.Exit(m.Run())
}

var now = time.Date(2026, 10, 12, 9, 0, 0, 0, time.FixedZone("AEDT", 11*3600))

func runningScenario() *scenario.Scenario {
	return &scenario.Scenario{
		ID: "adopt-running-01", Class: []string{"restraint/select"}, Now: now,
		Beats: []scenario.Beat{
			{Say: "I keep meaning to get back to running."},
			{Say: "Let's find a time."},
		},
		Card: []scenario.Entry{
			{ID: "why", When: "asked what it is for", Say: "Health, mostly."},
			{ID: "pick", When: "offered candidate placements for the running", Say: "The second one.",
				Licenses: []scenario.Licence{{Act: "select", Key: "running"}}},
		},
		Default: "I'm not sure yet.",
	}
}

// classifier answers by keyword, as a classifier model would.
func classifier(rules map[string]string) *testutil.Model {
	return &testutil.Model{Script: func(p anthropic.MessageNewParams) testutil.Reply {
		text, _ := testutil.LastUser(p)
		_, msg, _ := strings.Cut(text, "<message>")
		for k, v := range rules {
			if strings.Contains(msg, k) {
				return testutil.Reply{Text: v}
			}
		}
		return testutil.Reply{Text: "none"}
	}}
}

func config(t *testing.T, s *scenario.Scenario, agentModel, cls *testutil.Model, env ...string) episode.Config {
	t.Helper()
	return episode.Config{
		Impl: impl.Impl{Label: "fake", Command: fakeserver.Command(env...)}, Scenario: s,
		SeedDir: testutil.Seed(t), Workspace: filepath.Join(t.TempDir(), "workspace"),
		Agent:      agent.Settings{Model: "agent", MaxTokens: 1000, MaxTurns: 5},
		AgentModel: agentModel, Classifier: agent.Settings{Model: "classifier", MaxTokens: 10, MaxTurns: 1}, ClassifierModel: cls,
		MaxExchanges: 4, Now: time.Now,
	}
}

func TestEpisode(t *testing.T) {
	agentModel := &testutil.Model{Script: func(p anthropic.MessageNewParams) testutil.Reply {
		text, results := testutil.LastUser(p)
		switch {
		case results:
			return testutil.Reply{Text: "Done."}
		case text == "I keep meaning to get back to running.":
			return testutil.Reply{Tools: []testutil.ToolUse{
				{Name: "desire_add", Input: map[string]any{"title": "Get back to running"}},
				{Name: "intention_firm", Input: map[string]any{"id": testutil.TerminusID}},
			}}
		case text == "Let's find a time.":
			return testutil.Reply{Text: "Options: Wednesday 9:00, Wednesday 10:00, Thursday 14:00. Which do you want?"}
		case text == "The second one.":
			return testutil.Reply{Tools: []testutil.ToolUse{{Name: "select", Input: map[string]any{"id": testutil.TerminusID, "candidate": 2}}}}
		}
		t.Errorf("unexpected agent input %q", text)
		return testutil.Reply{Text: "?"}
	}}
	cls := classifier(map[string]string{"Which do you want?": "pick"})
	c := config(t, runningScenario(), agentModel, cls)
	rec, err := episode.Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Failure != "" {
		t.Fatalf("failure %s: %s", rec.FailureKind, rec.Failure)
	}
	if rec.ServerInfo.Name != "fake-intentions" {
		t.Errorf("server info %+v", rec.ServerInfo)
	}

	// The classified question is answered with the entry's text verbatim.
	var person []run.Message
	for _, m := range rec.Transcript {
		if m.Role == "person" {
			person = append(person, m)
		}
	}
	if len(person) != 3 || person[2].Kind != "entry" || person[2].Entry != "pick" || person[2].Text != "The second one." || person[2].Exchange != 3 {
		t.Errorf("person messages %+v", person)
	}

	// The licence enters the ledger when the entry is sent.
	if len(rec.Ledger) != 1 || rec.Ledger[0] != (run.Licence{Act: "select", Key: "running", Exchange: 3, From: "entry:pick"}) {
		t.Errorf("ledger %+v", rec.Ledger)
	}

	// Every call is logged at its exchange; the harness firm is refused.
	got := map[string]run.Call{}
	for _, call := range rec.Calls {
		got[call.Tool] = call
	}
	if c := got["desire_add"]; c.Exchange != 1 || c.IsError || c.ReadOnly {
		t.Errorf("desire_add %+v", c)
	}
	if c := got["intention_firm"]; c.Exchange != 1 || !c.Refused || c.Code != "refused" {
		t.Errorf("intention_firm not logged as refused: %+v", c)
	}
	if c := got["select"]; c.Exchange != 3 || c.IsError || !strings.Contains(string(c.Args), `"candidate":2`) {
		t.Errorf("select %+v", c)
	}

	// The phase prompt states the date and follows the server's instructions.
	first := agentModel.Calls[0]
	if len(first.System) != 2 || !strings.Contains(first.System[0].Text, "Harness intentions-bench") ||
		!strings.Contains(first.System[1].Text, "Monday 12 October 2026") {
		t.Errorf("system prompt %+v", first.System)
	}
	// The classifier saw only ids, when lines, and the agent's text.
	if n := len(rec.Classifications); n != 3 {
		t.Fatalf("%d classifications", n)
	}
	in := rec.Classifications[1].Input
	if !strings.Contains(in, "- pick: offered candidate placements for the running") || strings.Contains(in, "The second one.") || strings.Contains(in, "Health") {
		t.Errorf("classifier input leaks or misses the card:\n%s", in)
	}
	if _, err := os.Stat(filepath.Join(c.Workspace, "keys.yaml")); !os.IsNotExist(err) {
		t.Error("keys.yaml copied into the workspace")
	}
}

func TestOffCardAndCap(t *testing.T) {
	asked := 0
	agentModel := &testutil.Model{Script: func(p anthropic.MessageNewParams) testutil.Reply {
		asked++
		return testutil.Reply{Text: "Do you prefer mornings?"}
	}}
	cls := classifier(map[string]string{"mornings": "default"})
	s := runningScenario()
	s.Beats = s.Beats[:1]
	c := config(t, s, agentModel, cls)
	c.MaxExchanges = 2
	rec, err := episode.Run(context.Background(), c)
	if err != nil || rec.Failure != "" {
		t.Fatal(err, rec.Failure)
	}
	// One beat, two default answers, then the cap ends the scenario.
	if asked != 3 || len(rec.Classifications) != 2 {
		t.Errorf("agent turns %d, classifications %d", asked, len(rec.Classifications))
	}
	last := rec.Transcript[len(rec.Transcript)-2]
	if last.Kind != "default" || last.Text != "I'm not sure yet." {
		t.Errorf("default not sent: %+v", last)
	}
}

func TestMalformedClassifierReplyIsDefault(t *testing.T) {
	card := runningScenario().Card
	for raw, want := range map[string]string{"pick": "pick", " `PICK`.": "pick", "none": "none", "default": "default"} {
		if got, malformed := episode.ParseChoice(card, raw); got != want || malformed {
			t.Errorf("%q: %s %t", raw, got, malformed)
		}
	}
	if got, malformed := episode.ParseChoice(card, "I think they want the second option"); got != "default" || !malformed {
		t.Errorf("malformed reply: %s %t", got, malformed)
	}
}

func TestStartFailureKeepsStderr(t *testing.T) {
	c := config(t, runningScenario(), &testutil.Model{}, &testutil.Model{}, "FAKE_FAIL=no-licence")
	rec, err := episode.Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rec.FailureKind != "start" || !strings.Contains(rec.Stderr, "no-licence") {
		t.Errorf("start failure %q %q", rec.FailureKind, rec.Stderr)
	}
}

func TestClockCanary(t *testing.T) {
	agentModel := &testutil.Model{Script: func(p anthropic.MessageNewParams) testutil.Reply {
		if _, results := testutil.LastUser(p); results {
			return testutil.Reply{Text: "Noted."}
		}
		return testutil.Reply{Tools: []testutil.ToolUse{{Name: "desire_add", Input: map[string]any{"title": "Run"}}}}
	}}
	s := runningScenario()
	s.Beats = s.Beats[:1]
	c := config(t, s, agentModel, classifier(nil), "FAKE_WALL_CLOCK=1")
	rec, err := episode.Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if rec.FailureKind != "clock" || !strings.Contains(rec.Failure, "INTENTIONS_NOW") {
		t.Errorf("canary did not fire: %q %q", rec.FailureKind, rec.Failure)
	}
	// With the pinned clock the same run passes.
	c = config(t, s, agentModel, classifier(nil))
	if rec, _ := episode.Run(context.Background(), c); rec.Failure != "" {
		t.Errorf("pinned clock failed: %s", rec.Failure)
	}
}
