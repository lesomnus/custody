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

## How this repository was made

Four commands, and then the schema.

```sh
go tool pd new --setup github.com/lesomnus/custody custody
cd custody
go tool pd gen .
go mod tidy
```

`pd new` writes what a person writes and nothing a generator does: `go.mod`,
`buf.yaml`, `custody.yaml`, `cmd/{serve,config,auth}.go`, one binary, one example
entity, an overlay for payday's `Holder`, and a TypeScript package. `--setup`
then does the two things that need the network — `go get -tool` for the eight
tools it generates with, and `buf dep update`. It is a flag rather than the
default so that `pd new` works offline and prints the list instead.

`pd gen` is everything else: the service contracts, the messages and stubs, the
ent schema and its runtime, the CRUD servers, and the layers payday makes out of
the `(payday.entity)` options. It is run again after every change to the schema,
and `pd gen --check` in CI fails if a commit did not carry what it wrote.

One line of the template was changed before the first generation: `go_package`,
so that the messages land in `api/` rather than at the module root. See
[What is where](#what-is-where).

### And then what was written by hand

**About 1,200 lines, against 33,000 generated** — which is the number worth
having in a reference app, and most of the 1,200 is prose and tests.

| | |
| --- | --- |
| `proto/app/asset.proto` | the entity, in the place the template leaves `thing.proto`. The whole stack comes out of this one file |
| `proto/app/catalogue.proto` | a service written by hand, with no entity behind it |
| `proto/ext/app/asset_svc.ext.proto` | the overlay that adds `Transfer` to a generated contract |
| `policy.go` | `Hq` — 69 lines, and the whole of "headquarters sees every tenant" |
| `server/core/core.go` | the `Transfer` implementation |
| `server/catalogue/catalogue.go` | the public projection |
| `cmd/custody-admin/main.go` | the second binary. The first came from the template |
| `cmd/init.go` | the first tenant, through `Ungated`; on the admin binary only |
| `cmd/*_test.go` | three tests, and more than half the hand-written Go here |
| `ts/src/`, `ts/index.html`, `ts/admin/` | two pages: the customer UI and headquarters', and the local store's one line |

Everything else under `cmd/` came from the template and was edited rather than
written: `serve.go` is the stack spelled out, which payday deliberately does not
hide behind a `payday.Serve(cfg)`, so an app that stacks a layer of its own edits
it. Here that is two lines —

```go
stacked, err := app.Build(walled.WithWatch(w), core.Build(), pd.AuditBuild(), pd.GateBuild())
app.RegisterCatalogueServiceServer(g, catalogue.New(s.Ent))
```

— one putting `Transfer` in the walled stack, and one registering the catalogue
outside it, which is what makes the projection unwalled and is meant to be
visible in the file rather than implied by a flag.

`proto/ext/payday/holder.ext.proto` is the template's too, unchanged. It is the
example of adding a field to an entity payday ships.

## What is where

| | |
| --- | --- |
| `proto/app/*.proto` | **yours** — the entities, and `catalogue.proto`, a service written by hand |
| `proto/ext/**` | **yours** — the overlay that adds `Transfer` to a generated contract |
| `proto/**/*_svc.g.proto` | generated: the contract of an entity |
| `proto/payday/` | generated in whole: payday's own entities, copied in |
| `api/` | generated: the messages, the stubs, the query helpers |
| `internal/ent/`, `server/bare/`, `server/pd/`, `ts/gen/` | generated |
| `policy.go`, `server/core/`, `server/catalogue/`, `cmd/`, `ts/src/` | **yours** |

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

# The first tenant, and somebody in it. It cannot arrive over the API: a tenant
# is not put up from inside one, and that rule refuses headquarters too. What
# puts it there is `Server.Ungated`, reachable from this command and from
# nowhere a request can get to -- which is why the command is on the admin
# binary only.
go run ./cmd/custody-admin --config custody-admin.yaml init
go run ./cmd/custody-admin --config custody-admin.yaml init --tenant acme
go run ./cmd/custody-admin --config custody-admin.yaml init --tenant globex

# `init` prints the identifier of the tenant it made. Put the first one in
# `custody-admin.yaml` as `hq:`, then:
go run ./cmd/custody serve                                   # customers, :50051 and :8080
go run ./cmd/custody-admin --config custody-admin.yaml serve # headquarters, :50052 and :8081
```

`custody-admin` refuses to start without `hq:` in its configuration — an admin
server that does not know which tenant headquarters is refuses everybody, which
looks like a network problem.

Two configuration files and one app: `--config` names which. They are two
**deployments** rather than two programs, and a deployment's configuration is
its own — in a real one they are two machines each with a `custody.yaml` of its
own, and on one laptop they need different names.

### From the shell

There is a second listener on each, because a browser cannot speak gRPC. What
answers there is Connect, which is a POST with a JSON body, so it is also the
endpoint anything with an HTTP client can reach:

```sh
curl -sX POST http://localhost:8080/app.AssetService/List \
  -H 'Content-Type: application/json' -H 'Connect-Protocol-Version: 1' \
  -H 'Authorization: Plain @acme/admin' -d '{}'
```

The same call, four ways, which is the whole README in one table:

| | |
| --- | --- |
| `:8080` as `@acme/admin` | acme's assets |
| `:8080` as `@hq/admin` | **nothing** — that binary has no policy on it |
| `:8081` as `@hq/admin` | every tenant's assets |
| `:8081` as `@acme/admin` | `permission_denied: this server is not for you` |

And `app.CatalogueService/Search` with no `Authorization` header at all.

## The UI

```sh
npm install --prefix ts
go tool pd gen --ts .        # the messages and service descriptors, as TypeScript
npm run dev --prefix ts
```

`http://localhost:5173/` is the customer page and `http://localhost:5173/admin/`
is headquarters'.

**Two pages, and the split is not a security boundary.** That is worth saying
plainly, because the two binaries are one and it would be easy to read this as
the same thing. Anything a page can do, `curl` can do; the wall is on the
server. What two pages buy is that each has one address in it and shows the
screens that address has, and that the customer bundle carries no transfer form
for an endpoint it cannot reach.

They are two entries of one Vite package rather than two projects, because what
makes them two is which server they point at — and that is a build, not a
bundle. `npm run build --prefix ts` writes `dist/index.html` and
`dist/admin/index.html`, and a deployment serves the one it means to.

There is no framework. What these pages are for is to show that the generated
client is the whole API — that a screen is a call, and the call is the one
`curl` makes — and a framework would be the largest thing in the repository
saying nothing about that. `ts/src/client.ts` is thirty lines and does not grow
per service: protobuf-es emits the service descriptors and Connect's
`createClient` takes one.

Two things the pages show that the shell cannot:

- **the same request, two answers.** `AssetService.List` with no filter at all,
  on both pages. Nothing in either narrows it to a tenant and nothing could —
  the wall is a predicate in the query, put there by the server out of the
  credential.
- **what a `List` does not answer with.** The admin table shows which tenant
  each asset is in, and that is a second read per row with a `select` that names
  it: a list answers rows and not their edges.

## What is not here

**Real authentication.** Both binaries use payday's `Plain` handler, which
believes what the caller writes: it is for development and must not be served
where anybody can reach it. Making it real is `auth.Issuer` and `auth.Sessions`,
which payday leaves as a seam.

**A browser.** The UI type-checks, builds, and its client module has been run
against both live servers — the lists, the catalogue and a transfer all answer.
What has **not** happened is somebody loading the page in a browser, because
there is none here. So what is verified is that the pieces fit, and not that the
thing renders.

**The local store, and the reads that follow it.** payday ships both — a
replica of what a caller may see, and a query layer that knows which rows a
screen drew, so a row that changes redraws every place it appears.
`ts/src/store.ts` wires them and nothing calls it yet: these pages read the
server on every render, which is the right shape for two screens and the wrong
one for an app somebody has open all day.

Using it means a view framework, and that is the app's choice rather than
payday's — `@lesomnus/payday/react` is thirty lines of binding, and the same
file for Vue or Svelte is the same length.
