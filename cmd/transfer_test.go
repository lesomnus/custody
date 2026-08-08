package cmd_test

import (
	"testing"

	"github.com/google/uuid"
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

// TestTheTrailStaysWithTheTenantItLeft is payday's decision, demonstrated.
//
// `Audit` stamps the tenant of the **actor**, and nothing moves it. So the
// receiving tenant reads what has happened since the asset arrived and nothing
// before -- receiving something does not come with the right to read what its
// previous owner did inside their own walls.
func TestTheTrailStaysWithTheTenantItLeft(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v := b.asset(t, ctx, b.Acme, "lt-0001")
	k, err := pdid.From(v.GetId())
	x.NoError(err)

	// Something happens inside acme.
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

	// Every row about this asset is stamped with the tenant of whoever acted --
	// headquarters, or nobody for the write the deployment made itself before
	// there was anybody to be asking. Never the tenant that held it and never
	// the one that received it.
	var seen, byHq int
	for _, row := range rows {
		if pdid.Id(row.ObjectID) != k {
			continue
		}

		seen++
		switch row.TenantID {
		case b.Hq.Uuid():
			byHq++
		case uuid.Nil:
			// The setup, which nobody asked for.
		default:
			x.Failf("a trail row moved with the asset",
				"stamped %s, which is neither headquarters nor nobody", pdid.Id(row.TenantID))
		}
	}
	x.NotZero(seen, "the transfer was not recorded")
	x.NotZero(byHq, "the transfer headquarters made is not on the trail")
}
