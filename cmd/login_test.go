package cmd_test

import (
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/auth"

	"github.com/lesomnus/payday/config"
	"github.com/lesomnus/payday/pdid"
	"github.com/lesomnus/payday/pdtest"
	"github.com/lesomnus/payday/web"

	rcmd "github.com/lesomnus/roster/cmd"
	rstr "github.com/lesomnus/roster/rstr"
	rkeys "github.com/lesomnus/roster/server/keys"
	rvouch "github.com/lesomnus/roster/server/vouch"

	"github.com/lesomnus/custody/cmd"
)

// Somebody signing in to custody with a password roster holds.
//
// Both apps are real and the hop between them is a real one: roster answers on
// a listener, custody dials it. What this proves that no single-repository test
// can is that the pieces line up -- roster's answer is what custody's session
// needs, and custody's session is what its resolver can anchor.
//
// custody proves itself with a key roster's owner minted for it, and that key
// allows exactly `VouchService/Verify` -- so this also covers the thing an
// operator most wants to be true: the service that checks passwords cannot read
// or erase anybody.

// rostered is roster, on a listener, with one customer and one person in it --
// and a control plane holding the key custody calls it with.
func rostered(t *testing.T) (string, string, pdid.Id, pdid.Id) {
	t.Helper()
	x := require.New(t)
	ctx := t.Context()

	drv, dsn := pdtest.DB(t)
	cdrv, cdsn := pdtest.DB(t)

	s, err := rcmd.Build(ctx, rcmd.Config{
		Db:      config.DbConfig{Driver: drv, Dsn: dsn},
		Watch:   config.WatchConfig{Broker: config.BrokerMemory},
		Control: rcmd.ControlConfig{Db: config.DbConfig{Driver: cdrv, Dsn: cdsn}},
	})
	x.NoError(err)
	t.Cleanup(func() { s.Close() })
	x.NoError(s.Ent.Schema.Create(ctx))
	x.NoError(s.Control.Ent.Schema.Create(ctx))

	tenant := func(alias string) pdid.Id {
		v, err := s.Ungated.Tenant().Add(ctx, rstr.TenantAddRequest_builder{Alias: alias}.Build())
		x.NoError(err)
		k, err := pdid.From(v.GetId())
		x.NoError(err)

		return k
	}
	holder := func(in pdid.Id, alias string) pdid.Id {
		v, err := s.Ungated.Holder().Add(ctx, rstr.HolderAddRequest_builder{
			Tenant: rstr.TenantRef_builder{Id: in.Bytes()}.Build(),
			Alias:  alias,
		}.Build())
		x.NoError(err)
		k, err := pdid.From(v.GetId())
		x.NoError(err)

		return k
	}

	acme := tenant("acme")
	who := holder(acme, "someone")

	// And custody, in the **control plane**: a holder of the owner's one
	// tenant, with a key under it. `roster key add --service custody` is this,
	// and it makes what it needs on the way.
	svc, err := rcmd.ServiceOf(ctx, s.Control, "custody")
	x.NoError(err)

	token, sum, err := rkeys.Mint()
	x.NoError(err)

	_, err = s.Control.Ungated.ApiKey().Add(ctx, rstr.ApiKeyAddRequest_builder{
		Holder: rstr.HolderRef_builder{Id: svc.Bytes()}.Build(),
		Alias:  "production",

		// Exactly what custody needs and nothing else. Reading a person for a
		// screen is not in it yet, because no screen draws one.
		Methods: []string{"/roster.VouchService/Verify"},
		Secret:  sum,
	}.Build())
	x.NoError(err)

	// The password, set through the RPC that hashes it. Nothing else in either
	// app can write this column with a value that verifies.
	_, err = rvouch.New(s.Ungated, s.Ungated).Set(ctx, rstr.VouchSetRequest_builder{
		Who:    rstr.VouchWho_builder{Id: who.Bytes()}.Build(),
		Secret: []byte("correct horse battery staple"),
	}.Build())
	x.NoError(err)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	x.NoError(err)

	g := s.Grpc(ctx, rcmd.Config{})
	go func() { _ = g.Serve(l) }()
	t.Cleanup(g.Stop)

	return l.Addr().String(), token, acme, who
}

// custodying is custody, dialling that roster, on an HTTP listener with a jar
// in front of it -- which is what a browser is.
func custodying(t *testing.T, roster, token string) (*httptest.Server, *http.Client, *cmd.Server) {
	t.Helper()
	x := require.New(t)
	ctx := t.Context()

	drv, dsn := pdtest.DB(t)

	s, err := cmd.Build(ctx, cmd.Config{
		Db:     config.DbConfig{Driver: drv, Dsn: dsn},
		Watch:  config.WatchConfig{Broker: config.BrokerMemory},
		Roster: cmd.RosterConfig{Addr: roster, Token: token},
	})
	x.NoError(err)
	t.Cleanup(func() { s.Close() })
	x.NoError(s.Ent.Schema.Create(ctx))

	h, err := web.New(config.HttpConfig{AllowWeb: true}, s.Grpc(ctx, cmd.Config{}, nil))
	x.NoError(err)

	login := s.Roster.Login()
	h.Handle("POST /session", s.Sessions.Serve(login))
	h.Handle("DELETE /session", s.Sessions.Serve(login))

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	x.NoError(err)

	return srv, &http.Client{Jar: jar}, s
}

func signsIn(t *testing.T, c *http.Client, srv *httptest.Server, body string) int {
	t.Helper()

	res, err := c.Post(srv.URL+"/session", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()

	return res.StatusCode
}

func rpc(t *testing.T, c *http.Client, srv *httptest.Server, method, body string) (int, string) {
	t.Helper()
	x := require.New(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		srv.URL+method, strings.NewReader(body))
	x.NoError(err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")

	res, err := c.Do(req)
	x.NoError(err)
	defer res.Body.Close()

	out := make([]byte, 1<<16)
	n, _ := res.Body.Read(out)

	return res.StatusCode, string(out[:n])
}

// TestSomebodySignsInToCustodyWithAPasswordRosterHolds is the whole path.
func TestSomebodySignsInToCustodyWithAPasswordRosterHolds(t *testing.T) {
	x := require.New(t)

	addr, token, _, who := rostered(t)
	srv, c, s := custodying(t, addr, token)

	// Nothing here yet: custody has never seen this person, which is the fact
	// `OnDemand` is built around.
	n, err := s.Ent.Holder.Query().Count(t.Context())
	x.NoError(err)
	x.Equal(0, n)

	// Anonymous first, so what changes is the sign-in.
	code, _ := rpc(t, c, srv, "/app.AssetService/List", `{}`)
	x.Equal(http.StatusUnauthorized, code)

	x.Equal(http.StatusNoContent,
		signsIn(t, c, srv, `{"tenant":"acme","alias":"someone","password":"correct horse battery staple"}`))

	code, out := rpc(t, c, srv, "/app.AssetService/List", `{}`)
	x.Equal(http.StatusOK, code, out)

	// And the anchor was made, on the first call rather than at sign-in -- so
	// `date_created` here means *first seen in custody*.
	v, err := s.Ent.Holder.Query().Only(t.Context())
	x.NoError(err)
	x.Equal(who.String(), pdid.Id(v.ID).String())

	// Carrying nothing that can go stale. roster knows them as `someone`; here
	// the alias is seven characters payday made up, because a copy of roster's
	// name is a copy that is wrong the first time somebody marries.
	x.NotEqual("someone", v.Alias)
	x.NotEmpty(v.Alias)
}

// TestAWrongPasswordSignsNobodyIn, and leaves no anchor behind.
func TestAWrongPasswordSignsNobodyIn(t *testing.T) {
	x := require.New(t)

	addr, token, _, _ := rostered(t)
	srv, c, s := custodying(t, addr, token)

	x.Equal(http.StatusUnauthorized,
		signsIn(t, c, srv, `{"tenant":"acme","alias":"someone","password":"hunter2"}`))

	code, _ := rpc(t, c, srv, "/app.AssetService/List", `{}`)
	x.Equal(http.StatusUnauthorized, code)

	n, err := s.Ent.Holder.Query().Count(t.Context())
	x.NoError(err)
	x.Equal(0, n, "a refused sign-in made a row")
}

// TestSomebodyRosterHasNeverHeardOf, which answers the same as a wrong
// password: roster refuses to say which it was, and custody has nothing to add.
func TestSomebodyRosterHasNeverHeardOf(t *testing.T) {
	x := require.New(t)

	addr, token, _, _ := rostered(t)
	srv, c, _ := custodying(t, addr, token)

	x.Equal(http.StatusUnauthorized,
		signsIn(t, c, srv, `{"tenant":"acme","alias":"nobody","password":"correct horse battery staple"}`))
}

// TestSigningOutStopsTheNextCall.
func TestSigningOutStopsTheNextCall(t *testing.T) {
	x := require.New(t)

	addr, token, _, _ := rostered(t)
	srv, c, _ := custodying(t, addr, token)

	x.Equal(http.StatusNoContent,
		signsIn(t, c, srv, `{"tenant":"acme","alias":"someone","password":"correct horse battery staple"}`))

	code, _ := rpc(t, c, srv, "/app.AssetService/List", `{}`)
	x.Equal(http.StatusOK, code)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/session", nil)
	x.NoError(err)
	res, err := c.Do(req)
	x.NoError(err)
	res.Body.Close()

	code, _ = rpc(t, c, srv, "/app.AssetService/List", `{}`)
	x.Equal(http.StatusUnauthorized, code, "a signed-out browser was still served")
}

// TestTheSessionCarriesNoPassword, which is what custody keeps of one: nothing.
//
// The plaintext passes through this process on its way to roster and is not
// written anywhere -- there is no column for it in custody's schema, which is
// the strongest form of that guarantee.
func TestTheSessionCarriesNoPassword(t *testing.T) {
	x := require.New(t)

	addr, token, _, _ := rostered(t)
	srv, c, s := custodying(t, addr, token)

	x.Equal(http.StatusNoContent,
		signsIn(t, c, srv, `{"tenant":"acme","alias":"someone","password":"correct horse battery staple"}`))

	_, _ = rpc(t, c, srv, "/app.AssetService/List", `{}`)

	v, err := s.Ent.Holder.Query().Only(t.Context())
	x.NoError(err)
	x.NotContains(v.String(), "correct horse")

	// And the cookie is a handle and not a claim.
	for _, ck := range c.Jar.Cookies(mustURL(t, srv.URL)) {
		x.NotContains(ck.Value, "correct horse")
		x.NotContains(ck.Value, "someone")
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()

	u, err := url.Parse(s)
	require.NoError(t, err)

	return u
}

// TestCustodysKeyCannotReadAnybody is what an operator most wants to be true of
// the service that checks passwords.
//
// The key allows `VouchService/Verify` and nothing else, so custody can ask
// whether a password is right and cannot read a person, change one, or erase
// one -- even though it holds a credential for the store that could.
func TestCustodysKeyCannotReadAnybody(t *testing.T) {
	x := require.New(t)

	addr, token, _, who := rostered(t)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	x.NoError(err)
	t.Cleanup(func() { conn.Close() })

	ctx := auth.BearerProvider(token).Provide(t.Context())

	_, err = rstr.NewHolderServiceClient(conn).Get(ctx, rstr.HolderGetRequest_builder{
		Ref: rstr.HolderRef_builder{Id: who.Bytes()}.Build(),
	}.Build())
	x.Equal(codes.PermissionDenied, status.Code(err), "custody read a person")

	_, err = rstr.NewHolderServiceClient(conn).Erase(ctx,
		rstr.HolderRef_builder{Id: who.Bytes()}.Build())
	x.Equal(codes.PermissionDenied, status.Code(err), "custody erased somebody")
}
