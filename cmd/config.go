// Package cmd is this app's own wiring, and it is short on purpose.
//
// Everything that does not change from one app to the next is in payday. What
// is left is here, and it is deliberately **not** hidden behind a
// `payday.Serve(cfg)`: the stack, the order of the interceptors and which
// server the wall is on are the decisions a reader of an app most needs to be
// able to see, and a framework that hid them would be hiding the only part
// worth reading.
package cmd

import (
	"github.com/lesomnus/xli"

	"github.com/lesomnus/payday/gate"

	"github.com/lesomnus/payday/pdcmd"

	"github.com/lesomnus/payday/config"

	// The one driver this app runs on. It is blank-imported here rather than
	// by payday so that an app does not carry an engine it never opens.
	_ "github.com/lesomnus/payday/config/dbsqlite3"
)

// Name is what this app is called, and it is the only place it is written.
// The environment prefix and the names of the configuration files are derived
// from it -- APPTEST_DB_DSN, apptest.yaml -- so there is nothing to keep in
// step.
const Name = "custody"

// Loader reads this app's configuration.
var Loader = config.For(Name)

// Config is what this app is configured with.
//
// The framework cannot own this struct, since what an app is configured with is
// the app's. What it owns is the pieces: each of these is a payday type, and
// what is written here is only which of them this app has.
type Config struct {
	Server config.ServerConfig `yaml:"server"`
	Db     config.DbConfig     `yaml:"db"`
	Otel   config.OtelConfig   `yaml:"otel"`
	Watch  config.WatchConfig  `yaml:"watch"`

	// Hq is the headquarters tenant, by identifier, and is read only by
	// `custody-admin`.
	//
	// It is configuration rather than a value compiled in, and that is the
	// point: a privilege that belongs to whoever holds a particular row cannot
	// be revoked or narrowed, and one written in a deployment's own file can.
	Hq string `yaml:"hq"`
}

// Cmd is this app's own command line: what payday supplies, plus whatever the
// app has of its own.
//
// `config`, `config env` and `version` are payday's -- they are the commands
// that run against a **deployment** rather than against a checkout, and every
// one of them needs something only the app can hand over. `config env` is the
// clearest: listing the variables a deployment can set means walking this
// struct, and the struct is the app's.
//
// `serve` is not among them and will not be. It is the one command whose body
// is the stack -- which layers, in which order, with the wall on which server
// -- and that is the most important thing a reader of an app can see.
// `policy` is what makes the two binaries this app builds different, and it is
// a function rather than a value because it is read out of the configuration --
// which is not loaded until a command runs.
func Cmd(c *Config, policy Policy) *xli.Command {
	return &xli.Command{
		Name:  Name,
		Brief: "custody",

		Commands: []*xli.Command{
			pdcmd.NewCmdVersion(),
			pdcmd.NewCmdConfig(Loader, c),
			NewCmdServe(c, policy),
		},

		Handler: xli.RequireSubcommand(),
	}
}

// Policy answers with what this deployment narrows by, once the configuration
// has been read. Nil is no policy at all, which is the customer-facing server:
// `gate.Decide` then answers with the caller's own tenant, and there is nothing
// on that binary that answers anything wider.
type Policy func(*Config) (gate.Policy, error)
