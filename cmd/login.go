package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/lesomnus/payday/auth"
	"github.com/lesomnus/payday/auth/authsession"
	"github.com/lesomnus/payday/frame"
	"github.com/lesomnus/payday/pdid"

	rstr "github.com/lesomnus/roster/rstr"
)

// Roster is the store this deployment's people live in.
//
// # Why custody asks somebody else
//
// custody keeps no passwords, and the reason is the same one that makes its
// `Holder` rows anchors rather than copies: people belong to the organisation
// and not to one of its products. A password kept here would be a second one to
// change, a second one to leak and a second one to be out of step -- and the
// first time somebody left the company they would still be able to sign in to
// this.
//
// So the only thing custody knows how to do with a password is hand it to
// roster and be told yes or no. Nothing here hashes, compares, counts attempts
// or locks anything, and `server/vouch` in roster says why each of those has to
// happen in one place.
//
// # What custody does see
//
// The plaintext, in memory, on its way past. That is the cost of custody
// serving the sign-in form itself, and it is the thing an OIDC provider in
// front would remove -- there the Login App sees it and custody never does.
// Worth knowing rather than discovering: a deployment that cannot accept it
// wants `auth.Issuer`-shaped separation, which is Hydra, and this app can be
// configured for that instead.
type Roster struct {
	conn *grpc.ClientConn

	// as is how custody proves itself to roster. roster answers nothing
	// anonymously, so this is a credential and not a label.
	as auth.Provider
}

// DialRoster opens the connection, and does not check that anybody is there.
//
// A store that is down should not stop this app from starting: everything
// custody does for somebody already signed in works without it, and refusing to
// start would turn roster's outage into custody's. What fails is signing in,
// which is what actually depends on it.
func DialRoster(c RosterConfig) (*Roster, error) {
	if !c.Serves() {
		return nil, nil
	}

	// Cleartext, which is wrong anywhere the network is not. A key travels on
	// every call, so this needs TLS before it leaves one machine -- and that is
	// a deployment's to configure rather than something this can decide.
	conn, err := grpc.NewClient(c.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("roster: %w", err)
	}

	return &Roster{conn: conn, as: auth.BearerProvider(c.Token)}, nil
}

func (r *Roster) Close() error {
	if r == nil {
		return nil
	}

	return r.conn.Close()
}

// Login is the [authsession.Verify] this app signs people in with.
//
// It is the seam payday left and cannot fill: what a form contains is this
// app's, and checking it is roster's. Everything between the two is here, and
// it is short on purpose.
func (r *Roster) Login() authsession.Verify {
	return func(ctx context.Context, req *http.Request) (authsession.Session, error) {
		var body struct {
			Tenant   string `json:"tenant"`
			Alias    string `json:"alias"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(nil, req.Body, 4<<10)).Decode(&body); err != nil {
			return authsession.Session{}, fmt.Errorf("login: %w", err)
		}

		// The tenant travels with the name, because an alias is unique within
		// one and names somebody in every other. Which tenant a page is signing
		// in to is the page's to say -- a hostname, a field, a path -- and
		// custody takes it as given here.
		v, err := rstr.NewVouchServiceClient(r.conn).Verify(r.as.Provide(ctx),
			rstr.VouchVerifyRequest_builder{
				Who: rstr.VouchWho_builder{
					Tenant: body.Tenant,
					Alias:  body.Alias,
				}.Build(),
				Secret: []byte(body.Password),
			}.Build())
		if err != nil {
			// roster refused custody, or is not there. Either way it is not
			// this person's fault and it is not a wrong password.
			return authsession.Session{}, fmt.Errorf("roster: %w", err)
		}
		if !v.GetOk() {
			// One refusal for every way it can fail, which is roster's
			// decision and not one to unpick here: an unknown person and a
			// wrong password are one answer and take the same time.
			//
			// `locked_until` is set when an account is closed for a while, and
			// this deliberately does not pass it on yet -- a page that shows it
			// is a page that has been designed, and there is none.
			return authsession.Session{}, errors.New("no")
		}

		who, err := pdid.From(v.GetHolder())
		if err != nil {
			return authsession.Session{}, err
		}
		tenant, err := pdid.From(v.GetTenant())
		if err != nil {
			return authsession.Session{}, err
		}

		// The identifiers and nothing else. No name, no photo, no department --
		// those are roster's, read when there is a screen to draw, and a copy
		// of them in a session is a copy that goes stale.
		//
		// The row in **this** app does not exist yet, and is not made here:
		// `OnDemand` makes it on the first call that needs one, which keeps
		// "has this person ever used custody" a fact rather than a side effect
		// of a login form.
		return authsession.Session{
			Id:       who.String(),
			TenantId: tenant.String(),

			// What the session allows is everything the person does, which is
			// what a sign-in at the product's own form means. A narrower one is
			// what a token from an issuer carries, and that path is
			// `authoidc`'s.
			Grant: frame.Whole(),
		}, nil
	}
}
