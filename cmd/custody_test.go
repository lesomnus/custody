package cmd_test

import (
	"context"
	"testing"

	"github.com/lesomnus/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/config"
	"github.com/lesomnus/payday/frame"
	"github.com/lesomnus/payday/gate"
	"github.com/lesomnus/payday/pdid"

	"github.com/lesomnus/custody"
	app "github.com/lesomnus/custody/api"
	"github.com/lesomnus/custody/cmd"
	"github.com/lesomnus/custody/server/pd"
)

// built is an app with headquarters and two customers in it.
type built struct {
	*cmd.Server

	Hq       pdid.Id
	HqAdmin  pdid.Id
	Acme     pdid.Id
	AcmeUser pdid.Id
	Hooli    pdid.Id
}

func build(t *testing.T) (*built, context.Context) {
	t.Helper()
	x := require.New(t)
	ctx := t.Context()

	s, err := cmd.Build(ctx, cmd.Config{
		Db:    dbOf(t),
		Watch: config.WatchConfig{Broker: config.BrokerMemory},
	})
	x.NoError(err)
	t.Cleanup(func() { s.Close() })
	x.NoError(s.Ent.Schema.Create(ctx))

	b := &built{Server: s}

	tenant := func(alias string) pdid.Id {
		v, err := s.Ungated.Tenant().Add(ctx, app.TenantAddRequest_builder{Alias: alias}.Build())
		x.NoError(err)
		k, err := pdid.From(v.GetId())
		x.NoError(err)

		return k
	}
	holder := func(in pdid.Id, alias string) pdid.Id {
		v, err := s.Ungated.Holder().Add(ctx, app.HolderAddRequest_builder{
			Tenant: app.TenantRef_builder{Id: in.Bytes()}.Build(),
			Alias:  alias,
		}.Build())
		x.NoError(err)
		k, err := pdid.From(v.GetId())
		x.NoError(err)

		return k
	}

	b.Hq = tenant("hq")
	b.HqAdmin = holder(b.Hq, "admin")
	b.Acme = tenant("acme")
	b.AcmeUser = holder(b.Acme, "user")
	b.Hooli = tenant("hooli")

	return b, ctx
}

// as is a request from somebody, with the scope a customer-facing server would
// have given them: their own tenant and nothing else.
func (b *built) as(ctx context.Context, actor, tenant pdid.Id) context.Context {
	f := frame.New(actor, tenant, frame.Whole()).WithScope(frame.Only(tenant))

	return frame.Into(ctx, f)
}

// asHq is headquarters with the scope its own policy answers with.
func (b *built) asHq(ctx context.Context) context.Context {
	f := frame.New(b.HqAdmin, b.Hq, frame.Whole()).WithScope(frame.Everything)

	return frame.Into(ctx, f)
}

// asHqFor is headquarters working on one customer: the policy says every
// tenant and the credential names one, and the meet of those is the one.
func (b *built) asHqFor(ctx context.Context, tenant pdid.Id) context.Context {
	g := frame.Whole().In(tenant)
	f := frame.New(b.HqAdmin, b.Hq, g).WithScope(frame.Everything.Meet(g))

	return frame.Into(ctx, f)
}

func (b *built) asset(t *testing.T, ctx context.Context, in pdid.Id, alias string) *app.Asset {
	t.Helper()

	v, err := b.Ungated.Asset().Add(ctx, app.AssetAddRequest_builder{
		Tenant: app.TenantRef_builder{Id: in.Bytes()}.Build(),
		Alias:  alias,
	}.Build())
	require.NoError(t, err)

	return v
}

// TestACustomerSeesOnlyItsOwn is the wall, and it is here because everything
// below is about widening it.
func TestACustomerSeesOnlyItsOwn(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	mine := b.asset(t, ctx, b.Acme, "lt-0001")
	theirs := b.asset(t, ctx, b.Hooli, "lt-0002")

	got, err := b.Walled.Asset().Get(b.as(ctx, b.AcmeUser, b.Acme), app.AssetGetRequest_builder{
		Ref: app.AssetRef_builder{Id: mine.GetId()}.Build(),
	}.Build())
	x.NoError(err)
	x.Equal("lt-0001", got.GetAlias())

	_, err = b.Walled.Asset().Get(b.as(ctx, b.AcmeUser, b.Acme), app.AssetGetRequest_builder{
		Ref: app.AssetRef_builder{Id: theirs.GetId()}.Build(),
	}.Build())
	x.Equal(codes.NotFound, status.Code(err))
}

// TestHeadquartersSeesEveryTenant, which is what its policy answers with -- and
// it is still a scope rather than the wall being off.
func TestHeadquartersSeesEveryTenant(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	acme := b.asset(t, ctx, b.Acme, "lt-0001")
	hooli := b.asset(t, ctx, b.Hooli, "lt-0002")

	for _, v := range []*app.Asset{acme, hooli} {
		got, err := b.Walled.Asset().Get(b.asHq(ctx), app.AssetGetRequest_builder{
			Ref: app.AssetRef_builder{Id: v.GetId()}.Build(),
		}.Build())
		x.NoError(err)
		x.Equal(v.GetAlias(), got.GetAlias())
	}
}

// TestHeadquartersWorkingOnOneCustomerCannotTouchAnother is the half that makes
// the wide scope safe to have, and it is free: `Tenants.Meet` narrows "every
// tenant" to whatever the credential names.
//
// So an operator who opened a customer's screen cannot reach another by
// accident -- the request says which, and the wall does the rest.
func TestHeadquartersWorkingOnOneCustomerCannotTouchAnother(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	acme := b.asset(t, ctx, b.Acme, "lt-0001")
	hooli := b.asset(t, ctx, b.Hooli, "lt-0002")

	on := b.asHqFor(ctx, b.Acme)

	got, err := b.Walled.Asset().Get(on, app.AssetGetRequest_builder{
		Ref: app.AssetRef_builder{Id: acme.GetId()}.Build(),
	}.Build())
	x.NoError(err)
	x.Equal("lt-0001", got.GetAlias())

	_, err = b.Walled.Asset().Get(on, app.AssetGetRequest_builder{
		Ref: app.AssetRef_builder{Id: hooli.GetId()}.Build(),
	}.Build())
	x.Equal(codes.NotFound, status.Code(err), "a credential naming one customer reached another")
}

// TestTheHqPolicyRefusesEverybodyElse is the second lock on the admin server.
//
// It is reachable only from inside the company, and this is what makes the
// network being wrong different from the walls being open: a customer
// credential arriving there is told no before a row is read.
func TestTheHqPolicyRefusesEverybodyElse(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	p := custody.Hq{Tenant: b.Hq}

	x.NoError(p.May(ctx, gateCall(b.HqAdmin, b.Hq)))

	err := p.May(ctx, gateCall(b.AcmeUser, b.Acme))
	x.Equal(codes.PermissionDenied, status.Code(err))
}

// TestTheAssetNumberIsTheAlias, so `@acme/lt-0001` reaches the row and
// "  LT-0001 " is the same one.
func TestTheAssetNumberIsTheAlias(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v, err := b.Ungated.Asset().Add(ctx, app.AssetAddRequest_builder{
		Tenant: app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		Alias:  "  LT-0001 ",
	}.Build())
	x.NoError(err)
	x.Equal("lt-0001", v.GetAlias())

	got, err := b.Walled.Asset().Get(b.as(ctx, b.AcmeUser, b.Acme), app.AssetGetRequest_builder{
		Ref: app.AssetRef_builder{Slug: app.AssetRefBySlug_builder{
			Alias:  z.Ptr("lt-0001"),
			Tenant: app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		}.Build()}.Build(),
	}.Build())
	x.NoError(err)
	x.Equal(v.GetId(), got.GetId())
}

// TestAnAssetWithNoNumberIsGivenOne, because a laptop out of a box has no
// sticker yet and refusing to record it until it does is how a system gets
// routed around.
func TestAnAssetWithNoNumberIsGivenOne(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v, err := b.Ungated.Asset().Add(ctx, app.AssetAddRequest_builder{
		Tenant: app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
	}.Build())
	x.NoError(err)
	x.NotEmpty(v.GetAlias())

	k, err := pdid.From(v.GetId())
	x.NoError(err)
	x.Equal(pd.AssetDomain, k.Domain())
}

// gateCall is what an interceptor hands a policy.
func gateCall(actor, tenant pdid.Id) gate.Call {
	return gate.Call{Actor: actor, Tenant: tenant, Action: app.AssetService_Get_FullMethodName}
}
