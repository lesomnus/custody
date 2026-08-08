# custody

An asset manager built on [payday](https://github.com/lesomnus/payday), and a
worked example of the three things a multi-tenant app has to decide about who
sees what.

It manages a company's assets, tangible and not: a laptop, a licence, a domain
name, a machine on a factory floor. Headquarters manages customer tenants and
can move assets between them.

## What it is here to show

payday's [docs/TENANCY.md](https://github.com/lesomnus/payday/blob/main/docs/TENANCY.md)
argues that "who sees what" is three separate axes. This is each of them,
running.

| axis | here |
| --- | --- |
| a caller who sees more than their own tenant | `Hq` — headquarters, and `custody-admin` |
| a row that anybody may see part of | `CatalogueService` — a projection, not a wall with a hole |
| an operation with a rule of its own | `Asset.Transfer` |

### Headquarters is a policy, not an ungated server

`Hq.Where` answers `frame.Everything`, and that is still a **scope**: an
operator working on one customer carries a credential naming that customer, and
`frame.Tenants.Meet` turns "every tenant" into "that one". So the wall does its
usual work and a wrong screen cannot reach the wrong customer.

`Ungated` would not do — it has no wall at all, so it cannot narrow to one
customer, which is what an admin does all day.

### And it is a second binary

`cmd/custody` passes no policy. Nil is not a weaker policy; it is the absence of
one, so **that binary has no code path that answers "every tenant"** and a forged
headquarters credential arriving at the public address has nothing to call.
`cmd/custody-admin` passes `Hq` and is deployed where only the company can reach
it.

Two deployments is not a substitute for the policy. It is what makes the policy
unreachable from the internet.

### The catalogue is a different message

Making `Asset` publicly readable at the schema level publishes every field it
has, and nobody wants the location and the keeper published. So what is public
is a different shape of the same row -- which is a different message, which is a
different RPC. The walled path keeps no hole, and a field added to `Asset`
tomorrow is published only by somebody adding a line to the projection.

### Transfer is why the general writes are closed

`Asset.tenant` is the one tenant edge in this schema that is **not** immutable,
because a laptop really does move between subsidiaries and the row that names it
is the same row. That means `Patch` could set it -- which is exactly why `Patch`
and `Apply` are closed to callers. `Transfer` is where the rule lives, and
closing the general writes is what makes it the only door.

Three things happen and only the first is a write: the asset changes tenant, the
keeper is cleared (that person is in the tenant it left), and **the trail stays
behind**. `Audit` stamps the tenant of the actor and nothing moves it, so the
receiving tenant reads what has happened since the asset arrived and nothing
before. That is payday's decision, and this is the first thing to demonstrate
it.

## What is where

| | |
| --- | --- |
| `proto/app/*.proto` | **yours** — the entities, and `catalogue.proto`, a service written by hand |
| `proto/ext/**` | **yours** — the overlay that adds `Transfer` to a generated contract |
| `proto/**/*_svc.g.proto` | generated: the contract of an entity |
| `proto/payday/` | generated in whole: payday's own entities, copied in |
| `api/` | generated: the messages, the stubs, the query helpers |
| `internal/ent/`, `server/bare/`, `server/pd/` | generated |
| `policy.go`, `server/core/`, `server/catalogue/`, `cmd/` | **yours** |

So `.g` means a generator wrote it, `proto/payday/` is the one directory where
that is true of every file rather than of the ones marked, and `proto/ext/` is
excluded from the buf module because an overlay is a fragment rather than a file
that compiles.

The messages are in `api/` rather than at the module root, which is one line of
schema:

```proto
option go_package = "github.com/lesomnus/custody/api";
```

payday reads that rather than taking a flag, so it is said once and everything
follows from it. `internal/ent` and `server/bare` do not move with it — they are
named from the module root — and the top of this repository is the app rather
than a hundred `.pb.go`.

## Running it

```sh
go tool pd gen .                        # everything the schema says
go tool pd gen --check .                # and fail if a commit did not carry it
go run ./cmd/custody serve              # customers
go run ./cmd/custody-admin serve        # headquarters, behind the company network
```

`custody-admin` refuses to start without `hq:` in its configuration — an admin
server that does not know which tenant headquarters is refuses everybody, which
looks like a network problem.

## What is not here

Real authentication. Both binaries use payday's `Plain` handler, which believes
what the caller writes: it is for development and must not be served where
anybody can reach it. Making it real is `auth.Issuer` and `auth.Sessions`, which
payday leaves as a seam.
