// Command intentions-bench measures Intentions Format implementations, with
// the agent that uses them, on scripted scenarios.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/nodelogicau/intentions-bench/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], cli.DefaultDeps())
	stop()
	os.Exit(code)
}
