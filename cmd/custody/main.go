// Command custody is the server customers reach.
//
// # What is not in this binary
//
// A policy that answers "every tenant". `Grpc` is handed nil here, and nil is
// not a weaker policy -- it is the absence of one, and `gate.Decide` then
// answers with the caller's own tenant. So a headquarters credential arriving
// at this address is served exactly what any other credential is: its own
// tenant's rows. There is nothing wider to call.
//
// That is why the admin server is a second binary rather than a flag. A flag is
// a configuration mistake away from turning this into that one; a second main
// is a deployment.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/lesomnus/custody/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var c cmd.Config
	if err := cmd.Cmd(&c, nil).Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "custody:", err)
		os.Exit(1)
	}
}
