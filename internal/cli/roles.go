package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nodelogicau/agentloop/agent"
)

// apiKeyVars are the API key variables by provider, used unless a role names
// another with --api-key-env, --classifier-api-key-env or
// --judge-api-key-env.
var apiKeyVars = map[string]string{
	agent.ProviderAnthropic: "ANTHROPIC_API_KEY",
	agent.ProviderOpenAI:    "OPENAI_API_KEY",
}

// efforts are the efforts each provider accepts: Anthropic
// output_config.effort, OpenAI reasoning_effort. An empty effort is not sent.
var efforts = map[string][]string{
	agent.ProviderAnthropic: {"low", "medium", "high", "xhigh", "max"},
	agent.ProviderOpenAI:    {"none", "minimal", "low", "medium", "high", "xhigh", "max"},
}

// role is a model role: the agent and classifier, which run calls, and the
// judge, which score calls.
type role struct {
	name     string
	settings agent.Settings
	keyEnv   string
}

// roleFlags are a role's provider flags as given.
type roleFlags struct {
	provider, extraBody string
}

// configureRole validates a role's provider flags and applies the provider's
// defaults: on openai the effort is sent only when given and the base URL
// defaults to OpenAI's; on anthropic the provider field stays empty, so that
// recorded settings are as before providers existed.
func configureRole(prefix string, f roleFlags, s *agent.Settings, keyEnv *string, set map[string]bool) error {
	if _, ok := efforts[f.provider]; !ok {
		return fmt.Errorf("--%sprovider %q: use one of %v", prefix, f.provider, agent.Providers)
	}
	if f.provider == agent.ProviderOpenAI {
		s.Provider = f.provider
		if !set[prefix+"effort"] {
			s.Effort = ""
		}
		if s.BaseURL == "" {
			s.BaseURL = agent.DefaultOpenAIBaseURL
		}
	}
	s.BaseURL = strings.TrimRight(s.BaseURL, "/")
	if s.Effort != "" && !contains(efforts[f.provider], s.Effort) {
		return fmt.Errorf("--%seffort %q: %s accepts %v", prefix, s.Effort, f.provider, efforts[f.provider])
	}
	if f.extraBody != "" {
		if err := json.Unmarshal([]byte(f.extraBody), &s.ExtraBody); err != nil || s.ExtraBody == nil {
			return fmt.Errorf("--%sextra-body: not a JSON object: %s", prefix, f.extraBody)
		}
	}
	if *keyEnv == "" {
		*keyEnv = apiKeyVars[f.provider]
	}
	return nil
}

// judgeKeyEnv is the variable holding the judge's key in the score phase:
// --judge-api-key-env when given, else the default for the provider the run
// recorded for its judge.
func judgeKeyEnv(o *options, judge agent.Settings) string {
	if o.set["judge-api-key-env"] {
		return o.judgeKeyEnv
	}
	return apiKeyVars[judge.ProviderName()]
}

// describeRole names a role's provider, model, endpoint, parameters, and key
// variable, for the plan.
func describeRole(r role) string {
	s := r.settings
	base := s.BaseURL
	if base == "" {
		base = "SDK default"
	}
	var params []string
	if s.Effort != "" {
		params = append(params, "effort "+s.Effort)
	}
	if s.Temperature != nil {
		params = append(params, fmt.Sprintf("temperature %g", *s.Temperature))
	}
	if len(s.ExtraBody) > 0 {
		b, _ := json.Marshal(s.ExtraBody)
		params = append(params, "extra body "+string(b))
	}
	out := fmt.Sprintf("%s %s at %s, key %s", s.ProviderName(), s.Model, base, r.keyEnv)
	if len(params) > 0 {
		out += " (" + strings.Join(params, ", ") + ")"
	}
	return out
}
