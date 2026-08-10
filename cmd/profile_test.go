package cmd_test

import (
	"testing"

	"github.com/lesomnus/z"
	"github.com/stretchr/testify/require"

	"github.com/lesomnus/payday/auth"

	app "github.com/lesomnus/custody/api"
	"github.com/lesomnus/custody/cmd"
	"github.com/lesomnus/custody/internal/ent/holder"
)

// TestAProfileIsAnObjectOnTheHolder is a field on one of payday's entities that
// is not a scalar.
//
// It can be, and what comes out is a string column carrying the canonical
// protobuf JSON. Nothing about the overlay is special-cased for it: a message
// type in the numbers payday left free is merged like any other field.
//
// The assertion on the **row** is the one that matters. An Add answers with
// what it was given, so a value that was never stored still comes back looking
// right -- which is exactly how this went unnoticed until it was read again.
func TestAProfileIsAnObjectOnTheHolder(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v, err := b.Ungated.Holder().Add(ctx, app.HolderAddRequest_builder{
		Tenant: app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		Alias:  "with-a-profile",
		Profile: app.Profile_builder{
			DisplayName: "Ada Lovelace",
			Email:       "ada@acme.example",
			Locale:      "en-GB",
		}.Build(),
	}.Build())
	x.NoError(err)

	row, err := b.Ent.Holder.Query().Where(holder.Alias("with-a-profile")).Only(ctx)
	x.NoError(err)
	x.Equal("Ada Lovelace", row.Profile.GetDisplayName())
	x.Equal("en-GB", row.Profile.GetLocale())

	// And it is replaced whole, which is the shape it was chosen for: what a
	// provider said at one moment, not four fields that can half-apply.
	u, err := b.Ungated.Holder().Patch(ctx, app.HolderPatchRequest_builder{
		Ref:         v.Ref(),
		Profile:     app.Profile_builder{DisplayName: "Ada Byron"}.Build(),
		DateUpdated: v.GetDateUpdated(),
	}.Build())
	x.NoError(err)
	x.Equal("Ada Byron", u.GetProfile().GetDisplayName())

	// The locale is gone rather than kept, because the whole object was
	// assigned. That is the cost of the choice and it belongs here rather than
	// being discovered.
	x.Empty(u.GetProfile().GetLocale())
}

// TestAHolderWithNoProfileHasNone is the difference between "not set" and "set
// to nothing", which a column has to be able to answer.
func TestAHolderWithNoProfileHasNone(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	v, err := b.Ungated.Holder().Add(ctx, app.HolderAddRequest_builder{
		Tenant: app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		Alias:  "plain",
	}.Build())
	x.NoError(err)
	x.False(v.HasProfile())

	n, err := b.Ent.Holder.Query().
		Where(holder.Alias("plain"), holder.ProfileIsNil()).
		Count(ctx)
	x.NoError(err)
	x.Equal(1, n, "an unset profile is not NULL")
}

// TestTheProfileArrivesInTheFrame is the other half of the question: does
// anything have to be wired for a handler to see it.
//
// **No.** `frame.Frame.Row` already carries the actor as it was read, so a
// profile on the holder is in the frame the moment it is on the row -- no
// resolver change, no second query, nothing in `cmd/auth.go` to touch.
//
// Which also settles where a profile from an identity provider should be
// written, and it is not here: a resolver runs on **every** request rather
// than at login, and there is no frame yet to say who did the writing -- that
// is the thing it is in the middle of working out.
func TestTheProfileArrivesInTheFrame(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	ref := app.HolderRef_builder{Id: b.AcmeUser.Bytes()}.Build()

	was, err := b.Ungated.Holder().Get(ctx, app.HolderGetRequest_builder{Ref: ref}.Build())
	x.NoError(err)

	_, err = b.Ungated.Holder().Patch(ctx, app.HolderPatchRequest_builder{
		Ref:         ref,
		Profile:     app.Profile_builder{DisplayName: "A. User"}.Build(),
		DateUpdated: was.GetDateUpdated(),
	}.Build())
	x.NoError(err)

	f, err := cmd.Resolver(b.Ungated).Resolve(ctx, auth.Identity{Id: b.AcmeUser.String()})
	x.NoError(err)

	row, ok := f.Row.(*app.Holder)
	x.True(ok, "the frame carries the holder as it was read")
	x.Equal("A. User", row.GetProfile().GetDisplayName())
}

// TestTheScalarBesideItIsWhatGetsSearched is the cost of the object, stated as
// the thing to do instead.
//
// A claim that has to be looked up does not go in the profile: it goes flat,
// where it can be unique and indexed. That is what `idp_subject` is.
func TestTheScalarBesideItIsWhatGetsSearched(t *testing.T) {
	x := require.New(t)
	b, ctx := build(t)

	_, err := b.Ungated.Holder().Add(ctx, app.HolderAddRequest_builder{
		Tenant:     app.TenantRef_builder{Id: b.Acme.Bytes()}.Build(),
		Alias:      "findable",
		IdpSubject: z.Ptr("google-oauth2|107"),
		Profile:    app.Profile_builder{Email: "findable@acme.example"}.Build(),
	}.Build())
	x.NoError(err)

	// Unique, so two holders cannot be one person to the provider -- the
	// property a lookup key needs and a JSON value cannot have.
	_, err = b.Ungated.Holder().Add(ctx, app.HolderAddRequest_builder{
		Tenant:     app.TenantRef_builder{Id: b.Hooli.Bytes()}.Build(),
		Alias:      "same-person",
		IdpSubject: z.Ptr("google-oauth2|107"),
	}.Build())
	x.Error(err, "two holders took the same idp subject")
}
