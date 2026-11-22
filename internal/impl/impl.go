// Package impl starts an Intentions Format implementation from its command
// template and speaks MCP to it. The harness depends on nothing else an
// implementation provides.
//
// Starting, connecting and forwarding calls are agentloop's mcpclient. This
// package adds the intentions adapter contract: {workspace} is the seed
// copy, INTENTIONS_NOW pins the clock, and INTENTIONS_HARNESS and the MCP
// client name make every write a harness write.
package impl

import (
	"context"
	"path/filepath"
	"time"

	"github.com/nodelogicau/agentloop/mcpclient"
)

// Placeholder is replaced by the absolute workspace path in a command.
const Placeholder = "{workspace}"

// Harness is the MCP client name and the INTENTIONS_HARNESS value, so that
// every write the agent makes is attributed to a harness.
const Harness = "intentions-bench"

// Version is the harness version reported as the MCP client.
var Version = "dev"

// Impl is an implementation under test: a label and a command template.
type Impl = mcpclient.Impl

// ServerInfo identifies an implementation, from initialize.
type ServerInfo = mcpclient.ServerInfo

// StartError is a server that failed before initialize completed.
type StartError = mcpclient.StartError

// Server is a running implementation bound to one workspace.
type Server = mcpclient.Server

// Result is a tool call's outcome, with the error code the implementation
// gave for an error result.
type Result = mcpclient.Result

// Parse reads "<label>=<command>"; the command must contain {workspace}.
func Parse(s string) (Impl, error) { return mcpclient.Parse(s, Placeholder) }

// Start runs impl against workspace: {workspace} becomes the absolute path,
// which is also the working directory; INTENTIONS_NOW is set to now and
// INTENTIONS_HARNESS to Harness. It completes initialize and tools/list.
func Start(ctx context.Context, im Impl, workspace string, now time.Time) (*Server, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return mcpclient.Start(ctx, mcpclient.Command{
		Line: mcpclient.Expand(im.Command, Placeholder, abs),
		Dir:  abs,
		Env: []string{
			"INTENTIONS_NOW=" + now.Format(time.RFC3339),
			"INTENTIONS_HARNESS=" + Harness,
		},
	}, mcpclient.Client{Name: Harness, Version: Version})
}
