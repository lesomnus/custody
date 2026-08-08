// Command custody-admin is the server headquarters reaches, and it is deployed
// where only the company can.
//
// # It is the same app
//
// The same packages, the same stack, the same wall. One thing differs and it is
// one argument: the policy. Headquarters sees every tenant, and an operator
// working on one customer carries a credential narrowed to that customer, so
// the meet of the two is that customer -- the wall is doing the work it always
// does.
//
// # And it is a second binary on purpose
//
// A policy is a code path. Keeping it out of `custody` means the public address
// has nothing that answers "every tenant", whatever credential arrives there --
// which a flag on one binary cannot give, because a flag is a configuration
// mistake away from being set.
//
// It is a second lock and not the first: `custody.Hq` refuses anybody who is
// not headquarters before a row is read, so the network being wrong is not the
// same as the walls being open.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/lesomnus/payday/gate"
	"github.com/lesomnus/payday/pdid"

	"github.com/lesomnus/custody"
	"github.com/lesomnus/custody/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var c cmd.Config
	if err := cmd.Cmd(&c, hq).Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "custody-admin:", err)
		os.Exit(1)
	}
}

// hq reads which tenant headquarters is, and refuses to serve without one.
//
// An admin server whose headquarters is unset would be one where `May` refuses
// everybody, which serves nothing and looks like a network problem. Saying so
// at startup is the difference between a deployment that is wrong and a
// deployment that is wrong quietly.
func hq(c *cmd.Config) (gate.Policy, error) {
	if c.Hq == "" {
		return nil, fmt.Errorf("hq: say which tenant headquarters is; this server is for that one and refuses every other")
	}

	k, err := pdid.Parse(c.Hq)
	if err != nil {
		return nil, fmt.Errorf("hq: %w", err)
	}

	return custody.Hq{Tenant: k}, nil
}
