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
	"github.com/lesomnus/xli/flg"

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

	// Roster is where this deployment's people are, and where a password is
	// checked. Nothing written down is an app with no sign-in form of its own.
	Roster RosterConfig `yaml:"roster"`

	// Auth is where credentials come from, and saying nothing is `Plain` --
	// which believes whatever the caller writes.
	//
	// That default is deliberate and it is also the reason `Plain` announces
	// itself in the log: an app that could not be run until an identity
	// provider existed would be an app nobody runs, and one that is quietly
	// unauthenticated is worse. So it is easy and it is loud.
	Auth AuthConfig `yaml:"auth"`
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
//
// `more` is what one binary has and the other does not. It is a parameter
// rather than a flag on a shared command for the same reason the policy is:
// a command that is not on a binary cannot be reached on it, and a flag is a
// configuration mistake away from being set.
func Cmd(c *Config, policy Policy, more ...*xli.Command) *xli.Command {
	return &xli.Command{
		Name:  Name,
		Brief: "custody",

		Flags: flg.Flags{pdcmd.ConfigFlag()},

		Commands: append([]*xli.Command{
			pdcmd.NewCmdVersion(),
			pdcmd.NewCmdConfig(Loader, c),
			NewCmdServe(c, policy),
		}, more...),

		Handler: xli.Chain(pdcmd.Load(Loader, c), xli.RequireSubcommand()),
	}
}

// Policy answers with what this deployment narrows by, once the configuration
// has been read. Nil is no policy at all, which is the customer-facing server:
// `gate.Decide` then answers with the caller's own tenant, and there is nothing
// on that binary that answers anything wider.
type Policy func(*Config) (gate.Policy, error)

// AuthConfig is the identity provider this deployment trusts.
//
// An empty `issuer` is `Plain`: every caller is believed. Naming one turns on
// token verification -- signature against the provider's key set, issuer,
// audience, expiry -- and nothing else about the stack changes, because what
// reads a credential is a seam and everything behind it takes the same frame.
type AuthConfig struct {
	// Issuer is the provider, as the `iss` claim spells it. Empty is `Plain`.
	Issuer string `yaml:"issuer"`

	// Audience is what this app is called to that provider. Required when an
	// issuer is named: a verifier that skips it accepts a token minted for any
	// relying party of the same issuer.
	Audience string `yaml:"audience"`
}

// Serves reports whether this deployment verifies tokens rather than believing
// its callers.
func (c AuthConfig) Serves() bool { return c.Issuer != "" }

// RosterConfig is the store this deployment's people live in.
//
// Nothing written down is a deployment with no sign-in form of its own: custody
// then serves only callers arriving with a credential from somewhere else,
// which is what `auth.issuer` is for.
//
// # This is not deployable yet
//
// `As` is an `auth.Plain` name, so custody tells roster who it is and is
// believed. That is fine on one machine and an open door anywhere else: anybody
// who can reach roster can claim to be custody and then guess passwords at
// every tenant in the organisation.
//
// What replaces it is not a longer string. roster answers nothing anonymously,
// so custody needs a **row** there -- and what that row is has not been
// decided. `Holder` is a person, belongs to one tenant and is walled by it,
// while custody acts across every tenant it has users in. Whether the
// credential travels as a certificate or as an API key is the small half of
// that question. See roster's PLAN.md.
type RosterConfig struct {
	// Addr is where roster answers, e.g. "roster:50051".
	Addr string `yaml:"addr"`

	// As is what custody calls itself there, as `@tenant/alias`.
	As string `yaml:"as"`
}

// Serves reports whether this deployment can sign anybody in itself.
func (c RosterConfig) Serves() bool { return c.Addr != "" }
