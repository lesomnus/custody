package cmd_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/auth"
	"github.com/lesomnus/payday/config"
	"github.com/lesomnus/payday/pdid"
	"github.com/lesomnus/payday/pdtest"
	"github.com/lesomnus/payday/slug"

	app "github.com/lesomnus/custody/api"
	"github.com/lesomnus/custody/cmd"
	"github.com/lesomnus/custody/server/pd"
)

// TestSomebodyRosterKnowsAndCustodyDoesNot is the whole of what on-demand is
// for.
//
// The credential is verified before it gets here -- roster decided this person
// exists and which tenant holds them, and the signature is why that decision is
// trusted. custody has never seen them, and the row it needs is one it can make
// out of what the credential already carries.
func TestSomebodyRosterKnowsAndCustodyDoesNot(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	who := pdid.New(pd.HolderDomain)

	f, err := cmd.OnDemand(b.Ungated).Resolve(ctx, auth.Identity{
		Id:       who.String(),
		TenantId: b.Acme.String(),
	})
	x.NoError(err)
	x.Equal(who, f.Actor)
	x.Equal(b.Acme, f.Tenant)

	// And the row is here now, keyed by the identifier the credential named --
	// so it **is** that person rather than a row that maps to them.
	v, err := b.Ungated.Holder().Get(ctx, app.HolderGetRequest_builder{
		Ref: app.HolderRef_builder{Id: who.Bytes()}.Build(),
	}.Build())
	x.NoError(err)
	x.Equal(who.Bytes(), v.GetId())
}

// TestTheSecondVisitMakesNothing. It is an anchor, not a log: the row is the
// person, and there is one.
func TestTheSecondVisitMakesNothing(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	who := pdid.New(pd.HolderDomain)
	id := auth.Identity{Id: who.String(), TenantId: b.Acme.String()}

	for range 3 {
		_, err := cmd.OnDemand(b.Ungated).Resolve(ctx, id)
		x.NoError(err)
	}

	n, err := b.Ent.Holder.Query().Count(ctx)
	x.NoError(err)

	// Two from the harness, and one made here.
	x.Equal(3, n, "a repeat visit made another row")
}

// TestAnAnchorCarriesNothingThatCanGoStale.
//
// The name, the photo, the department are roster's, fetched when there is a
// screen to draw. What is here is the identifier and the tenant, and neither
// changes -- which is what makes this a cache that never needs invalidating.
func TestAnAnchorCarriesNothingThatCanGoStale(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	who := pdid.New(pd.HolderDomain)

	_, err := cmd.OnDemand(b.Ungated).Resolve(ctx, auth.Identity{
		Id:       who.String(),
		TenantId: b.Acme.String(),
	})
	x.NoError(err)

	v, err := b.Ungated.Holder().Get(ctx, app.HolderGetRequest_builder{
		Ref: app.HolderRef_builder{Id: who.Bytes()}.Build(),
	}.Build())
	x.NoError(err)

	// It has an alias, and nobody chose it. `Sink.WithNamer` is unset, so
	// payday folds what it was given and makes a name up when nothing was --
	// seven characters, which is what a server does with an Add that named
	// nothing.
	//
	// What matters is what it is **not**: roster's name for this person. That
	// is the copy this design exists to avoid, and it is the thing that would
	// go out of date the first time somebody marries.
	x.NotEmpty(v.GetAlias())
	x.NoError(slug.Validate(v.GetAlias()), "the alias is not one: %q", v.GetAlias())

	// `name` is the display name, and it stays empty here for the same reason.
	x.Empty(v.GetName())
	x.Empty(v.GetDesc())
}

// TestATenantCustodyHasNotSeenIsMade, which is the decision this app took.
//
// The tenant comes from a claim, so this means whoever signs tokens can create
// tenants here. That is correct rather than alarming: roster is the authority
// on which organisations exist, and the signature is what makes its answer
// trustworthy. The alternative is an operator adding every customer to every
// product by hand before anybody can sign in.
func TestATenantCustodyHasNotSeenIsMade(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	who := pdid.New(pd.HolderDomain)
	newTenant := pdid.New(pd.TenantDomain)

	f, err := cmd.OnDemand(b.Ungated).Resolve(ctx, auth.Identity{
		Id:       who.String(),
		Tenant:   "newcorp",
		TenantId: newTenant.String(),
	})
	x.NoError(err)
	x.Equal(newTenant, f.Tenant)

	v, err := b.Ungated.Tenant().Get(ctx, app.TenantGetRequest_builder{
		Ref: app.TenantRef_builder{Id: newTenant.Bytes()}.Build(),
	}.Build())
	x.NoError(err)
	x.Equal("newcorp", v.GetAlias())
}

// TestATenantNamedByIdentifierSurvivesARename is the reason to prefer the
// identifier over the alias, stated as the failure it avoids.
//
// An organisation renamed in roster and matched here by **alias** would arrive
// as a second tenant, and rows would split across the two with nothing failing.
// Matched by identifier it is the same tenant with a different name.
func TestATenantNamedByIdentifierSurvivesARename(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	who := pdid.New(pd.HolderDomain)

	_, err := cmd.OnDemand(b.Ungated).Resolve(ctx, auth.Identity{
		Id: who.String(), Tenant: "acme", TenantId: b.Acme.String(),
	})
	x.NoError(err)

	// The same tenant, renamed at the source.
	other := pdid.New(pd.HolderDomain)
	f, err := cmd.OnDemand(b.Ungated).Resolve(ctx, auth.Identity{
		Id: other.String(), Tenant: "acme-industries", TenantId: b.Acme.String(),
	})
	x.NoError(err)
	x.Equal(b.Acme, f.Tenant, "a rename made a second tenant")
}

// TestACredentialWithNoTenantIsRefused. There is nothing to anchor to, and
// guessing would put somebody in whichever tenant was convenient.
func TestACredentialWithNoTenantIsRefused(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	_, err := cmd.OnDemand(b.Ungated).Resolve(ctx, auth.Identity{
		Id: pdid.New(pd.HolderDomain).String(),
	})
	x.Error(err)
}

// TestAnAliasAloneWillNotAnchor.
//
// A resolver may find somebody by alias -- that is what `Plain` credentials
// look like -- but it may not **make** them by one. An alias is roster's name
// for a person and is not stable enough to key a row by: two people who rename
// past each other would end up sharing one.
func TestAnAliasAloneWillNotAnchor(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	_, err := cmd.OnDemand(b.Ungated).Resolve(ctx, auth.Identity{
		Tenant: "acme",
		Alias:  "nobody-here",
	})
	x.Error(err)
	x.NotEqual(codes.OK, status.Code(err))
}

// TestAnUnreachableProviderFailsToBuild is the fail-closed half.
//
// A deployment that named an issuer and cannot reach it must not fall back to
// believing its callers -- that would turn an outage at the provider into an
// open server, which is the one outcome worth being unable to reach by
// accident.
func TestAnUnreachableProviderFailsToBuild(t *testing.T) {
	x := require.New(t)

	_, err := cmd.Build(t.Context(), cmd.Config{
		Db:    dbOf(t),
		Watch: config.WatchConfig{Broker: config.BrokerMemory},
		Auth: cmd.AuthConfig{
			Issuer:   "https://nowhere.invalid",
			Audience: "custody",
		},
	})
	x.Error(err, "an unreachable provider built a server that believes everybody")
}

// TestNamingNoIssuerIsPlain, which is what a checkout does. It is easy on
// purpose and loud on purpose -- `auth.Plain` says so in the log once per
// process.
func TestNamingNoIssuerIsPlain(t *testing.T) {
	x := require.New(t)

	s, err := cmd.Build(t.Context(), cmd.Config{
		Db:    dbOf(t),
		Watch: config.WatchConfig{Broker: config.BrokerMemory},
	})
	x.NoError(err)
	t.Cleanup(func() { s.Close() })

	x.NotNil(s.Auth)
}

// dbOf is the database one test runs on: SQLite unless PDTEST_POSTGRES names
// another. Everything custody generates is SQL, and the two disagree in the
// directions that hide mistakes.
func dbOf(t *testing.T) config.DbConfig {
	t.Helper()

	drv, dsn := pdtest.DB(t)

	return config.DbConfig{Driver: drv, Dsn: dsn}
}
