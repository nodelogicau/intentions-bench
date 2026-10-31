// Package agent runs the tool-use loop against an MCP server through a model
// provider: the model sees the server's tools and instructions as served,
// every call is forwarded, and tokens and tool calls are counted.
//
// Copied from particulars-bench (internal/agent) at commit
// 770728513dc4f5a84030b047e86903b70662dfc2, without the OpenAI-compatible
// provider, and with Session added so that one conversation spans several
// user messages. Fixes made there must be ported by hand.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ProviderAnthropic is the only provider.
const ProviderAnthropic = "anthropic"

// ToolDef is a tool as the server lists it: name, description, and input
// schema. Each provider renders it in its own format, unchanged.
type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ToolCall is one tool call the model made. Args is the model's raw JSON,
// which need not be valid.
type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

// ToolResult answers one tool call.
type ToolResult struct {
	ID      string
	Text    string
	IsError bool
}

// Turn is one model response.
type Turn struct {
	Text      string
	ToolCalls []ToolCall
	// StopReason is the provider's own stop or finish reason.
	StopReason string
	// More reports that the model is waiting for its tool results.
	More bool
	// Truncated reports that the response stopped at the output limit.
	Truncated bool
	Usage     CallUsage
}

// Provider starts conversations with one model endpoint.
type Provider interface {
	Start(s Settings, system []string, user string, tools []ToolDef) (Conversation, error)
}

// Conversation is one loop's history, which the provider keeps in its own
// form so that it is replayed without conversion.
type Conversation interface {
	// Send requests the next response. A response the provider declined
	// to give returns its Turn with a *RefusalError.
	Send(ctx context.Context) (Turn, error)
	// AddToolResults answers the tool calls of the last response.
	AddToolResults(results []ToolResult)
	// AddUser appends a user message after the last response.
	AddUser(text string)
}

// Caller forwards a tool call to an implementation.
type Caller interface {
	Call(ctx context.Context, name string, args json.RawMessage) (text string, isError bool, err error)
}

// Settings are the model parameters every phase records.
type Settings struct {
	// BaseURL is empty for the SDK default.
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	MaxTokens int64  `json:"max_tokens"`
	MaxTurns  int    `json:"max_turns"`
	// Temperature is sent only when set.
	Temperature *float64 `json:"temperature,omitempty"`
}

// Usage is token use, summed and per call. InputTokens includes cached
// input, which is also broken out.
type Usage struct {
	Calls            int         `json:"calls"`
	InputTokens      int64       `json:"input_tokens"`
	OutputTokens     int64       `json:"output_tokens"`
	CacheReadTokens  int64       `json:"cache_read_tokens"`
	CacheWriteTokens int64       `json:"cache_write_tokens"`
	PerCall          []CallUsage `json:"per_call,omitempty"`
}

// CallUsage is one request's tokens, and the upstream that served it when
// the response names one.
type CallUsage struct {
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	Upstream         string `json:"upstream,omitempty"`
}

// Add accumulates u2 into u.
func (u *Usage) Add(u2 Usage) {
	u.Calls += u2.Calls
	u.InputTokens += u2.InputTokens
	u.OutputTokens += u2.OutputTokens
	u.CacheReadTokens += u2.CacheReadTokens
	u.CacheWriteTokens += u2.CacheWriteTokens
	u.PerCall = append(u.PerCall, u2.PerCall...)
}

func (u *Usage) record(c CallUsage) {
	u.Calls++
	u.InputTokens += c.InputTokens
	u.OutputTokens += c.OutputTokens
	u.CacheReadTokens += c.CacheReadTokens
	u.CacheWriteTokens += c.CacheWriteTokens
	u.PerCall = append(u.PerCall, c)
}

// Result is the outcome of one loop.
type Result struct {
	// Text is the text of the final response.
	Text       string `json:"text"`
	StopReason string `json:"stop_reason"`
	Turns      int    `json:"turns"`
	// Capped reports that the turn cap, or an output limit the provider
	// reports as truncation, ended the loop.
	Capped     bool           `json:"capped,omitempty"`
	ToolCalls  map[string]int `json:"tool_calls,omitempty"`
	ToolErrors int            `json:"tool_errors,omitempty"`
	Usage      Usage          `json:"usage"`
}

// RefusalError is a response the provider declined to give (Anthropic
// stop_reason "refusal").
type RefusalError struct{ Category, Explanation string }

func (e *RefusalError) Error() string {
	return fmt.Sprintf("model refused (%s): %s", e.Category, e.Explanation)
}

// Request is one loop's inputs.
type Request struct {
	Settings
	// Instructions is the server's initialize instructions, sent first in
	// the system prompt; Phase follows it.
	Instructions string
	Phase        string
	User         string
	Tools        []ToolDef
}

// Session is one conversation held across several user messages: each Send
// runs the tool-use loop for one message and the history carries over.
type Session struct {
	p    Provider
	c    Caller
	req  Request
	conv Conversation
}

// NewSession prepares a conversation; Request.User is ignored.
func NewSession(p Provider, c Caller, req Request) *Session {
	return &Session{p: p, c: c, req: req}
}

// Send adds a user message and drives the loop until the model stops
// calling tools or MaxTurns model calls have been made for this message.
func (s *Session) Send(ctx context.Context, user string) (Result, error) {
	if s.conv == nil {
		var system []string
		if s.req.Instructions != "" {
			system = append(system, s.req.Instructions)
		}
		if s.req.Phase != "" {
			system = append(system, s.req.Phase)
		}
		conv, err := s.p.Start(s.req.Settings, system, user, s.req.Tools)
		if err != nil {
			return Result{ToolCalls: map[string]int{}}, err
		}
		s.conv = conv
	} else {
		s.conv.AddUser(user)
	}
	return loop(ctx, s.conv, s.c, s.req.MaxTurns)
}

// Run drives the loop until the model stops calling tools or MaxTurns model
// calls have been made. Tool-level errors go back to the model as the server
// returned them, and arguments that are not valid JSON go back as an error
// result without reaching the server; a protocol error ends the loop with an
// error.
func Run(ctx context.Context, p Provider, c Caller, req Request) (Result, error) {
	return NewSession(p, c, req).Send(ctx, req.User)
}

func loop(ctx context.Context, conv Conversation, c Caller, maxTurns int) (Result, error) {
	res := Result{ToolCalls: map[string]int{}}
	for {
		if maxTurns > 0 && res.Turns >= maxTurns {
			res.Capped = true
			return res, nil
		}
		turn, err := conv.Send(ctx)
		var refusal *RefusalError
		if err != nil && !errors.As(err, &refusal) {
			return res, err
		}
		res.Turns++
		res.Usage.record(turn.Usage)
		res.StopReason = turn.StopReason
		if refusal != nil {
			return res, err
		}
		res.Text = strings.TrimSpace(turn.Text)
		if turn.Truncated {
			res.Capped = true
		}

		var results []ToolResult
		for _, call := range turn.ToolCalls {
			res.ToolCalls[call.Name]++
			if !json.Valid(call.Args) {
				res.ToolErrors++
				results = append(results, ToolResult{ID: call.ID, IsError: true,
					Text: fmt.Sprintf("invalid tool arguments: not a JSON value: %s", call.Args)})
				continue
			}
			out, isErr, err := c.Call(ctx, call.Name, call.Args)
			if err != nil {
				return res, fmt.Errorf("tool %s: %w", call.Name, err)
			}
			if isErr {
				res.ToolErrors++
			}
			results = append(results, ToolResult{ID: call.ID, Text: out, IsError: isErr})
		}
		// Every tool call is answered, even when the loop ends here, so
		// that a later user message continues a well-formed history.
		if len(results) > 0 {
			conv.AddToolResults(results)
		}
		if !turn.More || len(results) == 0 {
			return res, nil
		}
	}
}

// Complete sends a single request with no tools and returns its text.
func Complete(ctx context.Context, p Provider, s Settings, system, user string) (string, Usage, error) {
	res, err := Run(ctx, p, nil, Request{Settings: s, Phase: system, User: user})
	return res.Text, res.Usage, err
}

// New returns the provider for s, authenticated with apiKey.
func New(s Settings, apiKey string) (Provider, error) {
	return NewAnthropic(anthropicOptions(s, apiKey)...), nil
}
