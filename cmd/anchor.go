package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/lesomnus/z"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/auth"
	"github.com/lesomnus/payday/frame"
	"github.com/lesomnus/payday/pdid"

	app "github.com/lesomnus/custody/api"
)

// OnDemand is [Resolver] for a deployment whose people live in roster.
//
// # What a row here is
//
// An anchor, and not a copy. custody does not own people: roster does, and the
// token this deployment is handed already carries who somebody is. What custody
// needs locally is somewhere for `Asset.keeper` to point and somebody for the
// trail to name -- an identifier and a tenant, both of which the token holds.
//
// So the row carries **nothing that can go stale**. A name, a photo, a
// department are roster's, fetched when there is a screen to draw. What is here
// is the identifier, which never changes, and the tenant, which does not either.
//
// # It is created on first sight
//
// The absence of the row is exactly the fact "this person has not used custody
// before", which is worth being able to see -- and it makes `date_created` here
// mean *first seen in custody*, a different and useful fact from roster's *first
// known to the organisation*.
//
// The alternative is replicating roster's people ahead of time, which is a sync
// channel, a staleness window and a second origin for one fact.
//
// # Why this may write, when the resolver generally may not
//
// A resolver runs on every request and has no frame yet, so it is the wrong
// place for anything that happens repeatedly. This happens **once** per person,
// it is idempotent -- the identifier is the key -- and it is a write this app
// intends rather than a side effect.
//
// What it costs is that the trail cannot name who did it, since there is no
// frame to read. payday defines that case: `actor_id` is the zero identifier
// for "a write nobody asked for -- what a deployment does to itself". Which is
// what this is.
func OnDemand(s app.Server) auth.Resolver {
	known := Resolver(s)

	return auth.ResolverFunc(func(ctx context.Context, id auth.Identity) (*frame.Frame, error) {
		f, err := known.Resolve(ctx, id)
		if err == nil || !errors.Is(err, auth.ErrNoCredential) {
			return f, err
		}

		// Not here yet. Everything needed to make the anchor is in the
		// credential, and the credential has already been verified -- roster
		// decided this person exists and which tenant holds them, and the
		// signature is why that decision is trusted here.
		//
		// Read as `ErrNoCredential` and not as a `NotFound` code, which is what
		// this used to look for. `auth.Resolver` asks for that error for a
		// credential naming nobody who is here, so it is the contract rather
		// than a detail of how the call above happens to fail today.
		if err := anchor(ctx, s, id); err != nil {
			return nil, err
		}

		return known.Resolve(ctx, id)
	})
}

// anchor makes the local rows a verified credential implies.
func anchor(ctx context.Context, s app.Server, id auth.Identity) error {
	who, err := pdid.Parse(id.Id)
	if err != nil {
		// Only an identifier will do. An alias is roster's name for somebody
		// and is not stable enough to key a local row by -- and creating one
		// from a name would mean two people who renamed past each other end up
		// sharing a row.
		return fmt.Errorf("%w: an anchor needs an identifier: %s", auth.ErrNoCredential, err)
	}

	tenant, err := tenantOf(ctx, s, id)
	if err != nil {
		return err
	}

	_, err = s.Holder().Add(ctx, app.HolderAddRequest_builder{
		// The identifier the credential named, used as the key. payday's minter
		// takes what it is given and checks the domain, so this row **is** that
		// person here rather than a row that maps to them -- there is no
		// translation table and nothing to keep in step.
		Id:     who.Bytes(),
		Tenant: app.TenantRef_builder{Id: tenant.Bytes()}.Build(),

		// No alias, which is the whole of what to write here.
		//
		// A row nobody named has no name, and payday's answer to that is
		// already in the stack: `Sink.WithNamer` is unset, so `slug.Names`
		// folds what it was given and **makes one up when nothing was**. What
		// comes out is seven characters nobody chose.
		//
		// The one thing that must not go here is roster's name for them. That
		// is the copy this whole design is avoiding, and it would be a copy
		// that goes out of date the first time somebody marries.
	}.Build())
	if err != nil && status.Code(err) != codes.AlreadyExists {
		// AlreadyExists is two of somebody's requests arriving together, and
		// both wanting the same row. The one that lost is served by the row the
		// other made.
		return err
	}

	return nil
}

// tenantOf is the tenant the credential named, made here if this deployment has
// not seen it before.
//
// **Made rather than refused**, which is a decision worth being able to find.
// The tenant comes from a claim, so creating one means whoever signs tokens can
// create tenants here -- and that is correct: roster is the authority on which
// organisations exist, and the signature is what makes its answer trustworthy.
// Refusing would mean an operator has to add every customer to every product
// by hand before anybody can sign in.
//
// The cost is real and narrow: an organisation renamed in roster arrives as a
// **second** tenant here, and rows split across the two with nothing failing.
// Recovering is a merge. That is the argument for anchoring on an identifier
// rather than on an alias, and it is why this prefers `TenantId` when the
// credential carries one.
func tenantOf(ctx context.Context, s app.Server, id auth.Identity) (pdid.Id, error) {
	if id.TenantId != "" {
		k, err := pdid.Parse(id.TenantId)
		if err != nil {
			return pdid.Nil, fmt.Errorf("%w: %s", auth.ErrNoCredential, err)
		}

		if err := ensureTenant(ctx, s, k, id.Tenant); err != nil {
			return pdid.Nil, err
		}

		return k, nil
	}

	if id.Tenant == "" {
		return pdid.Nil, fmt.Errorf("%w: names no tenant", auth.ErrNoCredential)
	}

	v, err := s.Tenant().Get(ctx, app.TenantGetRequest_builder{
		Ref: app.TenantRef_builder{Alias: z.Ptr(id.Tenant)}.Build(),
	}.Build())
	if err == nil {
		return pdid.From(v.GetId())
	}
	if status.Code(err) != codes.NotFound {
		return pdid.Nil, err
	}

	// Named by alias only, so this deployment mints the identifier. The two
	// halves of the design disagree here and the alias wins by default: a
	// credential that names a tenant by identifier is the better shape, and
	// this is the fallback.
	v, err = s.Tenant().Add(ctx, app.TenantAddRequest_builder{Alias: id.Tenant}.Build())
	if err != nil {
		return pdid.Nil, err
	}

	return pdid.From(v.GetId())
}

// ensureTenant makes a tenant the credential named by identifier.
func ensureTenant(ctx context.Context, s app.Server, k pdid.Id, alias string) error {
	_, err := s.Tenant().Get(ctx, app.TenantGetRequest_builder{
		Ref: app.TenantRef_builder{Id: k.Bytes()}.Build(),
	}.Build())
	if err == nil {
		return nil
	}
	if status.Code(err) != codes.NotFound {
		return err
	}

	// No alias when the credential named none, rather than one made from the
	// identifier. A `pdid` written out is not an alias and payday refuses it --
	// it has to begin with a lowercase letter -- so that fallback was a tenant
	// that could never be created, found by a sign-in failing at 400 with a
	// message about hyphens.
	//
	// What replaces it is nothing at all: `Sink.WithNamer` is unset, so
	// `slug.Names` makes a name up when it was given none. The same answer the
	// holder below gets, for the same reason.
	_, err = s.Tenant().Add(ctx, app.TenantAddRequest_builder{
		Id:    k.Bytes(),
		Alias: alias,
	}.Build())
	if err != nil && status.Code(err) != codes.AlreadyExists {
		return err
	}

	return nil
}
