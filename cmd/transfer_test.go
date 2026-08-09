package cmd_test

import (
	"testing"

	"github.com/lesomnus/z"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/pdid"

	app "github.com/lesomnus/custody/api"
)

// TestHeadquartersMovesAnAssetBetweenTenants is the gimmick this project exists
// for, and every line of it collides with something payday decided.
func TestHeadquartersMovesAnAssetBetweenTenants(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v := b.asset(t, ctx, b.Acme, "lt-0001")

	got, err := b.Walled.Asset().Transfer(b.asHq(ctx), app.AssetTransferRequest_builder{
		Ref:    app.AssetRef_builder{Id: v.GetId()}.Build(),
		To:     app.TenantRef_builder{Id: b.Hooli.Bytes()}.Build(),
		Reason: z.Ptr("the Seoul office closed and the team moved"),
	}.Build())
	x.NoError(err)
	x.Equal(b.Hooli.Bytes(), got.GetTenant().GetId())

	// It left the wall it was behind. The tenant that had it cannot read it any
	// more, and is told NotFound rather than refused -- that it exists
	// somewhere is not this tenant's business.
	_, err = b.Walled.Asset().Get(b.as(ctx, b.AcmeUser, b.Acme), app.AssetGetRequest_builder{
		Ref: app.AssetRef_builder{Id: v.GetId()}.Build(),
	}.Build())
	x.Equal(codes.NotFound, status.Code(err))

	// And the identifier did not change, which is what makes it the same asset
	// rather than a new one that looks like it. Everything that named it -- a
	// trail row, a sticker, a spreadsheet -- still names it.
	x.Equal(v.GetId(), got.GetId())
}

// TestACustomerCannotHandAnAssetToAnother is the rule, and it is not written
// twice: it falls out of the scope.
//
// The destination is read through the wall, so a caller who cannot see that
// tenant is told NotFound. Headquarters sees every tenant and may transfer to
// any; a customer sees its own and cannot transfer anywhere.
func TestACustomerCannotHandAnAssetToAnother(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v := b.asset(t, ctx, b.Acme, "lt-0001")

	_, err := b.Walled.Asset().Transfer(b.as(ctx, b.AcmeUser, b.Acme), app.AssetTransferRequest_builder{
		Ref:    app.AssetRef_builder{Id: v.GetId()}.Build(),
		To:     app.TenantRef_builder{Id: b.Hooli.Bytes()}.Build(),
		Reason: z.Ptr("a customer helping itself to a transfer"),
	}.Build())
	x.Equal(codes.NotFound, status.Code(err))

	// And it did not move.
	got, err := b.Walled.Asset().Get(b.as(ctx, b.AcmeUser, b.Acme), app.AssetGetRequest_builder{
		Ref: app.AssetRef_builder{Id: v.GetId()}.Build(),
	}.Build())
	x.NoError(err)
	x.Equal(b.Acme.Bytes(), got.GetTenant().GetId())
}

// TestAnAssetNobodyCanSeeCannotBeTransferred: the asset is read through the
// wall too, so naming somebody else's is NotFound and not a refusal.
func TestAnAssetNobodyCanSeeCannotBeTransferred(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	theirs := b.asset(t, ctx, b.Hooli, "lt-0002")

	_, err := b.Walled.Asset().Transfer(b.as(ctx, b.AcmeUser, b.Acme), app.AssetTransferRequest_builder{
		Ref:    app.AssetRef_builder{Id: theirs.GetId()}.Build(),
		To:     app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		Reason: z.Ptr("helping myself to somebody else's laptop"),
	}.Build())
	x.Equal(codes.NotFound, status.Code(err))
}

// TestATransferSaysWhy, because the trail stays with the tenant the asset left
// and the sentence is what survives the move.
func TestATransferSaysWhy(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v := b.asset(t, ctx, b.Acme, "lt-0001")

	_, err := b.Walled.Asset().Transfer(b.asHq(ctx), app.AssetTransferRequest_builder{
		Ref: app.AssetRef_builder{Id: v.GetId()}.Build(),
		To:  app.TenantRef_builder{Id: b.Hooli.Bytes()}.Build(),
	}.Build())
	x.Equal(codes.InvalidArgument, status.Code(err))
	x.Contains(err.Error(), "reason")
}

// TestTheTrailIsFiledWithTheAssetAndNotTheActor is payday's decision,
// demonstrated -- and the decision changed, so this is what it is now.
//
// A trail row is filed under the tenant of the **thing that changed**, as it
// was when the write happened, with the actor's tenant beside it. It used to be
// filed under the actor's, and the case that broke was this one: headquarters
// acting on a customer's asset put the record behind headquarters' wall, where
// the customer could not see it, and their trail said nothing had happened.
//
// What survives the change is the property this test was written for. Nothing
// moves a row after it is stamped, so what acme did while it held the asset
// stays filed under acme -- receiving something does not come with the right to
// read what its previous owner did inside their own walls.
//
// What is different is the transfer itself. The row belongs to hooli by the
// time the write is over, so hooli reads "this arrived, and headquarters moved
// it", which is the one event about the asset they are a party to.
func TestTheTrailIsFiledWithTheAssetAndNotTheActor(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v := b.asset(t, ctx, b.Acme, "lt-0001")
	k, err := pdid.From(v.GetId())
	x.NoError(err)

	// Something happens inside acme, and is refused, so it writes nothing.
	_, err = b.Walled.Asset().Transfer(b.asHqFor(ctx, b.Acme), app.AssetTransferRequest_builder{
		Ref:    app.AssetRef_builder{Id: v.GetId()}.Build(),
		To:     app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		Reason: z.Ptr("this one is refused and writes nothing"),
	}.Build())
	x.Error(err, "transferring to where it already is")

	// And then it moves.
	_, err = b.Walled.Asset().Transfer(b.asHq(ctx), app.AssetTransferRequest_builder{
		Ref:    app.AssetRef_builder{Id: v.GetId()}.Build(),
		To:     app.TenantRef_builder{Id: b.Hooli.Bytes()}.Build(),
		Reason: z.Ptr("the Seoul office closed and the team moved"),
	}.Build())
	x.NoError(err)

	rows, err := b.Ent.Audit.Query().All(ctx)
	x.NoError(err)

	var seen, moved int
	for _, row := range rows {
		if pdid.Id(row.ObjectID) != k {
			continue
		}

		seen++
		switch row.TenantID {
		case b.Acme.Uuid():
			// Written while acme held it. The actor is whoever it was --
			// nobody, for the setup the deployment made before there was
			// anybody to be asking.
			x.NotEqual(b.Hooli.Uuid(), row.ActorTenantID)

		case b.Hooli.Uuid():
			// The transfer. Filed with the tenant that now holds the asset,
			// and carrying headquarters as the tenant whose operator did it --
			// so both parties read it and neither needs a scope wide enough to
			// see the other.
			moved++
			x.Equal(b.Hq.Uuid(), row.ActorTenantID,
				"the transfer does not say whose operator made it")

		default:
			x.Failf("a trail row is filed with neither tenant",
				"stamped %s", pdid.Id(row.TenantID))
		}
	}
	x.NotZero(seen, "the transfer was not recorded")
	x.Equal(1, moved, "the transfer is not filed with the tenant that received the asset")

	// And the whole of what hooli may read about this asset is that one event.
	// Everything acme did stays behind acme's wall.
	for _, row := range rows {
		if pdid.Id(row.ObjectID) != k || row.TenantID != b.Hooli.Uuid() {
			continue
		}
		x.Equal(b.Hq.Uuid(), row.ActorTenantID)
	}
}
