# Working on custody

A [payday](https://github.com/lesomnus/payday) app, and payday's reference app —
so it is also where payday's claims get tried against something real. If a
pattern here is awkward, that is worth reporting upstream rather than working
around.

`README.md` is the long version. Most of this app is **generated from
`proto/`**.

## Regenerate after touching the schema

```sh
go tool pd gen .          # messages, ent schema, servers, layers
go tool pd gen --ts .     # and the TypeScript half
go tool pd gen --check .  # what CI runs
go tool pd doctor .       # what would go wrong, before it does
```

A generated file that was not regenerated **compiles perfectly and is wrong**.
If you edited anything under `proto/`, you are not done until `pd gen --check`
exits 0.

```sh
go test ./...
cd ts && npm run check && npm test
```

## Do not edit — regenerate

`api/*.pb.go`, `api/*.g.go`, `server/bare/`, `server/pd/`, `internal/ent/`,
`proto/payday/`, `proto/**/*_svc.g.proto`, `ts/gen/`.

The generated messages are in **`api/`**, not at the module root — that is
`option go_package` in `proto/app/*.proto`, and every entity has to name the
same one.

To add a field to one of payday's entities (`Tenant`, `Holder`, `Audit`,
`Outbox`), write an overlay in `proto/ext/payday/`. Editing `proto/payday/`
directly is undone by the next generation.

## What is genuinely this app's

Four things, and they are worth reading before changing anything:

| | |
| --- | --- |
| `policy.go` | who may see whose rows — the one decision that is not generated |
| `server/core/` | this app's rules, including `Asset.Transfer` |
| `server/catalogue/` | the public projection. **The only place that reads without the wall** |
| `cmd/serve.go` | the stack: which layers, in which order, which server the wall is on |

## Two deployments, not one binary with a flag

```
cmd/custody         hands Grpc a nil policy    own tenant only. No "see everything" path exists here
cmd/custody-admin   hands it custody.Hq        internal-only, and Hq refuses anyone who is not HQ
```

nil is not a weaker policy — it is the **absence** of one, and `gate.Decide`
then answers the caller's own tenant and nothing else.

`custody.yaml` and `custody-admin.yaml` are the two configurations.

This is deliberate and it is the thing to preserve. In a single binary with a
policy that grants everything to HQ, the code path that returns every tenant's
rows **exists inside the public server** and only a credential check keeps it
shut. Here it is not at that address at all.

**So: do not add a "see all tenants" path to `cmd/custody`, and do not collapse
the two into one binary with a flag.** A configuration mistake must not be able
to turn the public server into the admin one.

## `server/catalogue` reads without the wall

That is its whole point and it is the only such place. What makes it safe is
that it publishes a **different message** — a projection with the fields that
may be public, not `Asset` with the wall off.

If you add a field to `Asset`, it is **not** published unless somebody writes it
into the projection. Keep it that way: publishing by default is how a location
or a keeper leaks.

## Writing a layer

Embed `api.Overlay`, and write `WithDriver` — nothing inherits it:

```go
func (s Core) WithDriver(drv dialect.Driver) (api.Server, error) {
	next, err := enttx.Rebind(s.Next(), drv)
	if err != nil {
		return nil, err
	}

	return New(next), nil
}

var (
	_ api.Server               = Core{}
	_ enttx.Binder[api.Server] = Core{}
)
```

Leave it out and nothing fails until a transaction is opened, because
`enttx.Rebind` asks at run time. `pd doctor` finds it.

`server/core/core.go` is the shape to copy.

## `Ungated`

`cmd/serve.go` builds `Walled` and `Ungated`. `Ungated` is not a privilege — it
is an instance the wall was never installed on, for work that cannot happen
inside a tenant (`init`, resolving who is calling).

**Never hand it to anything a caller can reach.**

## Upgrading payday

```sh
go get -u github.com/lesomnus/payday
go tool pd gen .
```

payday owns part of this app's schema, so a field added upstream arrives in
`internal/ent` here on the next generation. Two things refuse rather than
trusting anyone to remember: `pd.NewSink` rejects a binary linking a different
payday than the generated code came from, and `serve` refuses a database that
is not the shape `internal/ent` describes.

The migration is this app's to write — payday's entities and this app's are one
database and one ent client.

## Reference

- <https://github.com/lesomnus/payday/tree/main/docs> — the guides and the
  references behind them
- payday's own checkout is at `/workspace` when working on both
