package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/nodelogicau/agentloop/agent"
)

func parse(t *testing.T, args ...string) *options {
	t.Helper()
	o, err := parseFlags("run", args, io.Discard)
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return o
}

func TestAnthropicDefaultsRecordNoProvider(t *testing.T) {
	o := parse(t)
	for _, r := range append(o.runRoles(), role{"judge", o.judge, o.judgeKeyEnv}) {
		if r.settings.Provider != "" || r.settings.BaseURL != "" || r.keyEnv != "ANTHROPIC_API_KEY" {
			t.Errorf("%s: %+v, key %s", r.name, r.settings, r.keyEnv)
		}
	}
	if o.agent.Effort != "medium" || o.classifier.Effort != "low" {
		t.Errorf("efforts %q %q", o.agent.Effort, o.classifier.Effort)
	}
}

func TestOpenAIRoleDefaults(t *testing.T) {
	o := parse(t, "--provider", "openai", "--model", "gpt-5", "--classifier-provider", "openai", "--classifier-effort", "minimal",
		"--classifier-base-url", "https://openrouter.ai/api/v1/", "--classifier-api-key-env", "OPENROUTER_API_KEY",
		"--classifier-extra-body", `{"provider":{"order":["x"],"allow_fallbacks":false}}`)
	if o.agent.Provider != agent.ProviderOpenAI || o.agent.BaseURL != agent.DefaultOpenAIBaseURL || o.agentKeyEnv != "OPENAI_API_KEY" {
		t.Errorf("agent %+v key %s", o.agent, o.agentKeyEnv)
	}
	// The default effort is Anthropic's; on openai it is sent only when given.
	if o.agent.Effort != "" {
		t.Errorf("agent effort %q sent without --effort", o.agent.Effort)
	}
	c := o.classifier
	if c.Effort != "minimal" || c.BaseURL != "https://openrouter.ai/api/v1" || o.classifierKeyEnv != "OPENROUTER_API_KEY" {
		t.Errorf("classifier %+v key %s", c, o.classifierKeyEnv)
	}
	if p, ok := c.ExtraBody["provider"].(map[string]any); !ok || p["allow_fallbacks"] != false {
		t.Errorf("extra body %v", c.ExtraBody)
	}
	// The judge was left on anthropic.
	if o.judge.Provider != "" || o.judgeKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("judge %+v key %s", o.judge, o.judgeKeyEnv)
	}
}

func TestRoleFlagErrors(t *testing.T) {
	for args, want := range map[string]string{
		"--provider gemini":                            "--provider \"gemini\"",
		"--effort minimal":                             "anthropic accepts",
		"--judge-provider openai --judge-effort ultra": "--judge-effort \"ultra\": openai accepts",
		"--classifier-extra-body [1]":                  "--classifier-extra-body: not a JSON object",
	} {
		_, err := parseFlags("run", strings.Fields(args), io.Discard)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", args, err, want)
		}
	}
}

func TestJudgeKeyFollowsTheRecordedProvider(t *testing.T) {
	o := parse(t)
	if got := judgeKeyEnv(o, agent.Settings{Provider: agent.ProviderOpenAI}); got != "OPENAI_API_KEY" {
		t.Errorf("recorded openai judge: %s", got)
	}
	if got := judgeKeyEnv(o, agent.Settings{}); got != "ANTHROPIC_API_KEY" {
		t.Errorf("recorded anthropic judge: %s", got)
	}
	o = parse(t, "--judge-api-key-env", "MY_JUDGE_KEY")
	if got := judgeKeyEnv(o, agent.Settings{Provider: agent.ProviderOpenAI}); got != "MY_JUDGE_KEY" {
		t.Errorf("--judge-api-key-env ignored: %s", got)
	}
}

func TestDescribeRole(t *testing.T) {
	o := parse(t, "--classifier-provider", "openai", "--classifier-model", "m", "--classifier-extra-body", `{"a":1}`)
	got := describeRole(o.runRoles()[1])
	if got != `openai m at https://api.openai.com/v1, key OPENAI_API_KEY (extra body {"a":1})` {
		t.Errorf("%s", got)
	}
	if got := describeRole(o.runRoles()[0]); got != "anthropic claude-opus-5-5 at SDK default, key ANTHROPIC_API_KEY (effort medium)" {
		t.Errorf("%s", got)
	}
}
