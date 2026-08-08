// Package custody is this app's generated types, and the one decision that is
// not generated: who may see whose rows.
package custody

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/frame"
	"github.com/lesomnus/payday/gate"
	"github.com/lesomnus/payday/pdid"
)

// Hq is the policy the **admin** deployment runs with: the headquarters tenant
// sees every tenant there is, and nobody else reaches this server at all.
//
// # Why this is a policy and not an ungated server
//
// `Ungated` is what a deployment holds to do its own work, and it has no wall
// -- so it cannot narrow to one customer, which is the thing an admin does all
// day. This is a scope like any other: wide, and still a scope. Headquarters
// acting for one customer carries a credential narrowed to that customer, and
// `frame.Tenants.Meet` turns "every tenant" into "that one".
//
// # Why it is a separate deployment as well
//
// Because a policy is a code path, and the customer-facing binary should not
// have one that answers "every tenant". `cmd/custody` builds its stack with no
// policy at all -- `gate.Decide` then answers with the caller's own tenant and
// nothing else -- so a forged headquarters credential arriving there has
// nothing to call. Two deployments is not a substitute for the policy; it is
// what makes the policy unreachable from the internet.
type Hq struct {
	// Tenant is the headquarters, by identifier.
	//
	// It is configuration rather than a well-known value compiled in: a
	// privilege that belongs to whoever holds a particular row cannot be
	// revoked or narrowed, and one that is written in a deployment's own file
	// can be.
	Tenant pdid.Id
}

var _ gate.Policy = Hq{}

// May refuses everybody who is not headquarters.
//
// The admin deployment is reachable only from inside the company, and this is
// the second lock rather than the first: a customer credential that somehow
// arrives here is told no before any row is read, so the network being wrong is
// not the same as the walls being open.
func (p Hq) May(ctx context.Context, c gate.Call) error {
	if c.Tenant == p.Tenant {
		return nil
	}

	return status.Error(codes.PermissionDenied, "this server is not for you")
}

// Where answers with every tenant there is.
//
// It is still narrowed by whatever the credential says: a session opened for
// one customer carries a grant naming that customer, and the meet of "every
// tenant" and "that one" is that one. So an operator working on a customer
// cannot touch another by accident -- the scope of the request says which.
func (p Hq) Where(ctx context.Context, c gate.Call) (frame.Tenants, error) {
	return frame.Everything, nil
}
