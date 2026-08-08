// Package catalogue is the one thing this app serves to nobody in particular.
//
// # Why it is not the asset with the wall off
//
// Making `Asset` publicly readable at the schema level would publish every
// field it has, and nobody wants the location and the keeper published. So what
// is public is a different shape of the same row -- and a different shape is a
// different message, which is a different RPC.
//
// What that buys, and it is worth being concrete:
//
//   - the walled path keeps no hole. `Asset.Get` narrows as it always did and
//     there is no flag anywhere that turns it off;
//   - the projection is written out below, so a field added to `Asset` tomorrow
//     is not published by having been forgotten;
//   - and "which rows are public" is this app's predicate, in this app's code.
//
// # What it costs
//
// This is the only place in the app that reads without the wall, and it does it
// by talking to ent rather than to a generated server. That is deliberate: a
// read with no caller has no scope to narrow by, so there is nothing for the
// wall to do, and pretending otherwise by handing it an empty frame would make
// the next reader wonder which rows it was hiding.
//
// What is not free is the discipline. Every predicate here is this file's, so
// the file is short and stays short.
package catalogue

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/custody/api"
	"github.com/lesomnus/custody/internal/ent"
	"github.com/lesomnus/custody/internal/ent/asset"
)

// Size is how many items a page holds when the caller did not say, and Max is
// the most it will ever hold.
//
// There is a cap for the reason every generated List has one: a page a caller
// can make arbitrarily large is a read a caller can make arbitrarily expensive,
// and this one is reachable by anybody at all.
const (
	Size = 20
	Max  = 100
)

// Server answers the public catalogue.
type Server struct {
	api.UnimplementedCatalogueServiceServer

	db *ent.Client
}

func New(db *ent.Client) *Server { return &Server{db: db} }

// Search answers with the assets that are listed, newest last.
//
// Two narrowings and both are this file's: `listed`, which is what the owning
// tenant said may be published, and the page. There is no third -- a caller has
// no identity here, so there is nothing else to narrow by.
func (s *Server) Search(ctx context.Context, req *api.CatalogueSearchRequest) (*api.CatalogueSearchResponse, error) {
	size := int(req.GetSize())
	switch {
	case size <= 0:
		size = Size
	case size > Max:
		return nil, status.Errorf(codes.InvalidArgument,
			"size: %d is more than the %d this serves at a time", size, Max)
	}

	q := s.db.Asset.Query().
		Where(asset.ListedEQ(true)).
		Order(ent.Asc(asset.FieldDateCreated), ent.Asc(asset.FieldID)).
		Limit(size + 1)

	vs, err := q.All(ctx)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "the catalogue cannot be read just now")
	}

	// One more than the page was asked for, which is how it knows there is a
	// next one without counting the table.
	next := ""
	if len(vs) > size {
		vs = vs[:size]
		next = vs[len(vs)-1].ID.String()
	}

	items := make([]*api.CatalogueItem, len(vs))
	for i, v := range vs {
		// The projection, written out. A field added to Asset tomorrow is not
		// published by having been forgotten -- it is published by somebody
		// adding a line here, which is a thing a reviewer sees.
		items[i] = api.CatalogueItem_builder{
			Id:    v.ID[:],
			Alias: v.Alias,
			Name:  v.Name,
			Desc:  v.Desc,
		}.Build()
	}

	return api.CatalogueSearchResponse_builder{Items: items, Next: next}.Build(), nil
}
