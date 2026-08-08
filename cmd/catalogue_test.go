package cmd_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	app "github.com/lesomnus/custody/api"
	"github.com/lesomnus/custody/server/catalogue"
)

// TestTheCatalogueIsAProjectionAndNotAHole is the whole argument of that
// package, as a test.
//
// Making `Asset` publicly readable at the schema level would publish every
// field it has. What is public here is a **different message**, and the test of
// that is not that the wall was bypassed -- it is that the fields nobody should
// see are not in the answer at all.
func TestTheCatalogueIsAProjectionAndNotAHole(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v, err := b.Ungated.Asset().Add(ctx, app.AssetAddRequest_builder{
		Tenant:   app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		Alias:    "lt-0001",
		Name:     "ThinkPad X1",
		Desc:     "on loan",
		Location: "Seoul HQ 7F",
		Listed:   true,
	}.Build())
	x.NoError(err)

	res, err := catalogue.New(b.Ent).Search(ctx, app.CatalogueSearchRequest_builder{}.Build())
	x.NoError(err)
	x.Len(res.GetItems(), 1)

	got := res.GetItems()[0]
	x.Equal(v.GetId(), got.GetId())
	x.Equal("lt-0001", got.GetAlias())
	x.Equal("ThinkPad X1", got.GetName())

	// And there is nowhere in the answer for the location or the keeper to be.
	// That is not a filter somebody remembered to apply -- the message has no
	// field for them, so a field added to Asset tomorrow is published only by
	// somebody adding a line to the projection.
	x.NotContains(got.String(), "Seoul")
}

// TestOnlyWhatTheOwnerListedIsPublic.
func TestOnlyWhatTheOwnerListedIsPublic(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	for _, tt := range []struct {
		alias  string
		listed bool
	}{{"lt-0001", true}, {"lt-0002", false}} {
		_, err := b.Ungated.Asset().Add(ctx, app.AssetAddRequest_builder{
			Tenant: app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
			Alias:  tt.alias,
			Listed: tt.listed,
		}.Build())
		x.NoError(err)
	}

	res, err := catalogue.New(b.Ent).Search(ctx, app.CatalogueSearchRequest_builder{}.Build())
	x.NoError(err)
	x.Len(res.GetItems(), 1)
	x.Equal("lt-0001", res.GetItems()[0].GetAlias())
}

// TestItIsCrossTenant, which is the thing a policy could never express: the
// answer is about the rows rather than about who is asking.
func TestItIsCrossTenant(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	for _, in := range []struct {
		tenant string
		alias  string
	}{{"acme", "lt-0001"}, {"hooli", "lt-0002"}} {
		k := b.Acme
		if in.tenant == "hooli" {
			k = b.Hooli
		}

		_, err := b.Ungated.Asset().Add(ctx, app.AssetAddRequest_builder{
			Tenant: app.TenantRef_builder{Id: k.Bytes()}.Build(),
			Alias:  in.alias,
			Listed: true,
		}.Build())
		x.NoError(err)
	}

	res, err := catalogue.New(b.Ent).Search(ctx, app.CatalogueSearchRequest_builder{}.Build())
	x.NoError(err)
	x.Len(res.GetItems(), 2, "the catalogue is one tenant's")
}

// TestAPageHasACap, because this one is reachable by anybody at all.
func TestAPageHasACap(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	_, err := catalogue.New(b.Ent).Search(ctx, app.CatalogueSearchRequest_builder{
		Size: 1000,
	}.Build())
	x.Equal(codes.InvalidArgument, status.Code(err))
}
