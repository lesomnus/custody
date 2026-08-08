package cmd

import (
	"context"
	"fmt"

	"github.com/lesomnus/xli"
	"github.com/lesomnus/xli/flg"

	"github.com/lesomnus/payday/pdid"

	api "github.com/lesomnus/custody/api"
)

// NewCmdInit is `custody-admin init`: the first tenant, and somebody in it.
//
// It exists because there is nowhere else it could happen. A tenant is not put
// up from inside one -- that rule is a layer rather than a wall, and it refuses
// **everybody**, headquarters included -- so the first row of a deployment
// cannot arrive over the API. What puts it there is `Server.Ungated`, which is
// not a privilege anybody holds: it is a server instance this process was
// handed, reachable from this command and from nowhere a request can get to.
//
// It is on `custody-admin` and not on `custody` for the same reason the policy
// is. The public binary has no code path that writes outside a tenant, so a
// deployment cannot be talked into making one.
//
// Running it twice is an error rather than a no-op: an alias is unique, so the
// second run is refused by the database. That is the right answer -- an `init`
// that quietly did nothing is one somebody runs against the wrong deployment
// and believes.
func NewCmdInit(c *Config) *xli.Command {
	return &xli.Command{
		Name:  "init",
		Brief: "put up the first tenant and somebody in it",

		Flags: flg.Flags{
			&flg.String{Name: "tenant", Brief: "the alias of the tenant to create"},
			&flg.String{Name: "holder", Brief: "the alias of the holder to create in it"},
		},

		Handler: xli.OnRun(func(ctx context.Context, cmd *xli.Command, next xli.Next) error {
			tenant, ok := flg.Find[string](cmd, "tenant")
			if !ok || tenant == "" {
				tenant = "hq"
			}

			holder, ok := flg.Find[string](cmd, "holder")
			if !ok || holder == "" {
				holder = "admin"
			}

			s, err := Build(ctx, *c)
			if err != nil {
				return err
			}
			defer s.Close()

			// The schema, so that a fresh database is one this can run against.
			// A deployment with migrations of its own does that instead; see
			// payday's `migrate`.
			if err := s.Ent.Schema.Create(ctx); err != nil {
				return err
			}

			t, err := s.Ungated.Tenant().Add(ctx, api.TenantAddRequest_builder{
				Alias: tenant,
			}.Build())
			if err != nil {
				return fmt.Errorf("tenant %q: %w", tenant, err)
			}

			h, err := s.Ungated.Holder().Add(ctx, api.HolderAddRequest_builder{
				Tenant: api.TenantRef_builder{Id: t.GetId()}.Build(),
				Alias:  holder,
			}.Build())
			if err != nil {
				return fmt.Errorf("holder %q: %w", holder, err)
			}

			k, err := pdid.From(t.GetId())
			if err != nil {
				return err
			}

			// The identifier rather than the alias, because that is what goes
			// in `hq:` -- a policy that named a row by a name somebody can
			// change is a policy that changes hands.
			cmd.Printf("tenant %s is %s\n", tenant, k)
			cmd.Printf("holder %s is %s\n", holder, must(pdid.From(h.GetId())))
			cmd.Printf("\nput this in the admin server's configuration:\n\n    hq: %s\n\n", k)
			cmd.Printf("and sign in as: @%s/%s\n", tenant, holder)

			return nil
		}),
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}

	return v
}
