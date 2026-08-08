// Package core is what this app writes by hand.
//
// payday generates the CRUD of every entity and refuses to serve a general
// write. What is left is an operation that means something -- one with a rule
// nothing else has -- and there is exactly one of those here.
//
// It is a layer in the same sense every other one is: a `struct{ api.Overlay }`
// that answers the RPCs it has something to say about and hands the rest down.
// So it stacks with the wall, the gate and the trail rather than standing beside
// them, and a `Transfer` is on the trail for the same reason an `Add` is --
// nothing listed it.
package core

import (
	"context"

	"entgo.io/ent/dialect"
	"github.com/protobuf-orm/protoc-gen-orm-ent/runtime/enttx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/gate"
	"github.com/lesomnus/payday/pderr"

	"github.com/lesomnus/custody/api"
)

// Core is the layer that answers what this app wrote.
type Core struct {
	api.Overlay
}

func New(next api.Server) Core { return Core{api.NewOverlay(next)} }

// Build makes a builder of this layer so that it can be stacked.
func Build() api.Builder { return builder{} }

type builder struct{}

func (builder) Build(next api.Server) (api.Server, error) { return New(next), nil }

var (
	_ api.Server               = Core{}
	_ enttx.Binder[api.Server] = Core{}
)

// WithDriver answers with this stack running on `drv`.
//
// Every layer writes this and none can inherit it: an overlay holds what is
// behind it and has no way to make itself again, so a layer that did not write
// it would be missing from the rebuilt stack and the requests inside the
// transaction would go around it.
func (s Core) WithDriver(drv dialect.Driver) (api.Server, error) {
	next, err := enttx.Rebind(s.Next(), drv)
	if err != nil {
		return nil, err
	}

	return New(next), nil
}

type coreAsset struct {
	Core
	api.AssetServiceServer
}

func (s Core) Asset() api.AssetServiceServer {
	return coreAsset{s, s.Next().Asset()}
}

// Transfer moves an asset from the tenant that holds it to another.
//
// # What the wall does and does not do here
//
// Reading the asset is narrowed like every other read, so a caller who cannot
// see it is told NotFound -- that it exists is itself something not to say.
//
// The **destination** is not a narrowing. The row has not moved, so there is
// nothing yet to filter; whether this caller may hand an asset to that tenant
// is a decision about a row that does not exist in that shape yet. payday has
// the same shape in its own gate -- "a holder is added to a tenant you can
// see" -- and the answer is the same: read the tenant through the wall, and
// let NotFound be the answer when it is not visible.
//
// So headquarters, whose policy answers with every tenant, may transfer to any
// of them; a customer may transfer only to a tenant it can already see, which
// for an ordinary customer is itself, which is not a transfer. That is the rule
// falling out of the scope rather than being written twice.
func (s coreAsset) Transfer(ctx context.Context, req *api.AssetTransferRequest) (*api.Asset, error) {
	if _, err := gate.Actor(ctx); err != nil {
		return nil, err
	}

	// A reason is required, because a transfer nobody can explain a year later
	// is the one thing a trail cannot reconstruct: the row is in its new tenant
	// and the trail is in the old one.
	if _, err := reason(req.GetReason()); err != nil {
		return nil, err
	}

	to, err := s.Core.Next().Tenant().Get(ctx, api.TenantGetRequest_builder{
		Ref: req.GetTo(),
	}.Build())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, gate.ErrNotFound("Tenant")
		}

		return nil, err
	}

	// Read through the wall, so this is also the check that the caller holds it.
	v, err := s.AssetServiceServer.Get(ctx, api.AssetGetRequest_builder{
		Ref: req.GetRef(),
	}.Build())
	if err != nil {
		return nil, err
	}
	if string(v.GetTenant().GetId()) == string(to.GetId()) {
		return nil, pderr.Invalidf("to", "it is already there")
	}

	// One write, through the servers below -- so the trail, the watch and the
	// outbox are told exactly as they are for anything else.
	//
	// The keeper goes with it, and that is a decision rather than an omission:
	// being answerable for something does not travel, because the person is in
	// the tenant the asset left.
	return s.AssetServiceServer.Patch(ctx, api.AssetPatchRequest_builder{
		Ref:         api.AssetRef_builder{Id: v.GetId()}.Build(),
		Tenant:      req.GetTo(),
		DateUpdated: v.GetDateUpdated(),
	}.Build())
}

// reason is the sentence that survives the move.
func reason(v string) (string, error) {
	if len(v) < 8 {
		return "", pderr.Invalidf("reason",
			"say why in a sentence somebody will understand in a year; the trail stays with the tenant this leaves")
	}

	return v, nil
}
