package cmd_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	rstr "github.com/lesomnus/roster/rstr"
	rkeys "github.com/lesomnus/roster/server/keys"
)

// TestATokenRosterIssuedWorksHere is the whole point of `payday.TokenService`:
// a string custody cannot read, presented to custody, and custody asking whoever
// issued it.
//
// A session cookie is between a browser and the app that set it. This is what
// somebody pastes into a script, and until now custody had no way to make sense
// of one.
func TestATokenRosterIssuedWorksHere(t *testing.T) {
	x := require.New(t)
	ctx := t.Context()

	r := rosterUp(t)

	// A key for the person, in roster's **data plane** -- `rt_`, which resolves
	// to the holder rather than to the key.
	token, sum, err := rkeys.Mint(rkeys.PrefixTenant)
	x.NoError(err)

	_, err = r.Ungated.ApiKey().Add(ctx, rstr.ApiKeyAddRequest_builder{
		Holder:  rstr.HolderRef_builder{Id: r.Who.Bytes()}.Build(),
		Alias:   "a-script",
		Secret:  sum,
		Methods: []string{"/app.AssetService/List"},
	}.Build())
	x.NoError(err)

	// custody presents its own key to ask, and that key allows
	// `TokenService/Introspect` -- see `rostered`. That allowance is the whole
	// of the trust decision and is one an operator makes per app.
	srv, c, _ := custodying(t, r.Addr, r.Token)

	bearing := func(method, tok string) int {
		t.Helper()
		x := require.New(t)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			srv.URL+method, strings.NewReader(`{}`))
		x.NoError(err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		req.Header.Set("Authorization", "Bearer "+tok)

		res, err := c.Do(req)
		x.NoError(err)
		defer res.Body.Close()

		return res.StatusCode
	}

	t.Run("the token names the person", func(t *testing.T) {
		require.Equal(t, http.StatusOK, bearing("/app.AssetService/List", token))
	})

	// What it was narrowed to takes away, on custody's chain, from whatever
	// custody's own policy decided. The strings are **custody's** method names,
	// which roster stored without ever having heard of them -- and could not
	// have checked, since it has no descriptors for another app.
	t.Run("and not what it was not made for", func(t *testing.T) {
		require.Equal(t, http.StatusForbidden, bearing("/app.AssetService/Add", token))
	})

	t.Run("a token nobody minted is refused", func(t *testing.T) {
		require.Equal(t, http.StatusUnauthorized,
			bearing("/app.AssetService/List", "rt_nothingatall"))
	})
}
