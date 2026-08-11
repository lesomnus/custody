package cmd

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/lesomnus/otx/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/lesomnus/payday/auth"
	"github.com/lesomnus/payday/pdid"

	rstr "github.com/lesomnus/roster/rstr"
)

// Asking roster whether somebody is still there.
//
// # What this is instead of
//
// A streaming `Holder.Watch` used to do this, and it was the wrong shape three
// times over. `Watch` is what a **page** uses to keep a handful of rows
// current: it names the rows it is about, at most thirty-two of them, and its
// filters are refused when one names a row that is gone. custody wanted the
// opposite -- every person it has ever anchored, and specifically the news that
// one of them is no longer there.
//
// So it broke at the thirty-third person to sign in, and again at the first
// departure of anybody, and both failures were a warning line every five
// seconds while sessions quietly lasted their full twelve hours.
//
// # Why a check on the request path is enough
//
// A session is checked against custody's own store on every request, so ending
// one takes effect immediately. What custody lacks is not enforcement, it is
// **news**. So this asks for the news on a timer instead of subscribing to it:
//
//   - the cost is one RPC per person per [Fresh.Every], not per request, and
//     not per person in the directory -- only the ones with a live session
//   - there is no stream to re-open, nothing to fall behind, and no filter that
//     can name a row into an error
//   - it is bounded by **active sessions**, which is the number that matters
//
// The window it leaves is `Every`. That is the same window every OAuth
// deployment lives with as an access-token lifetime, and it is a floor rather
// than the only line: `authsession`'s idle clock ends an unused session
// regardless.
//
// # It does not fail closed
//
// roster being unreachable does not sign everybody out. The last answer stands
// until the next check, because refusing would turn roster's outage into
// custody's -- and the risk in that window is the one this whole design already
// accepts between checks.
type Fresh struct {
	roster *Roster

	// Every is how long an answer about somebody is trusted for.
	Every time.Duration

	mu   sync.Mutex
	seen map[string]time.Time
}

func NewFresh(r *Roster) *Fresh {
	return &Fresh{roster: r, Every: time.Minute, seen: map[string]time.Time{}}
}

// Wrap is `h`, with a check that whoever it names still exists.
//
// It goes **outside** the session handler and not inside the resolver, because
// what it is checking is the credential rather than the row: a session naming
// somebody roster has erased is a credential that is no longer good, which is
// what `auth` means by a credential that is present and wrong.
func (f *Fresh) Wrap(h auth.Handler) auth.Handler {
	if f == nil || f.roster == nil {
		return h
	}

	return auth.HandlerFunc(func(ctx context.Context) (auth.Identity, error) {
		id, err := h.Handle(ctx)
		if err != nil || id.Id == "" {
			return id, err
		}
		if f.fresh(id.Id) {
			return id, nil
		}

		switch err := f.ask(ctx, id.Id); {
		case err == nil:
			return id, nil

		case status.Code(err) == codes.NotFound:
			// Erased, or moved out of what custody's key may see. Either way
			// this credential names nobody roster will answer for, and serving
			// it would be serving somebody who has left.
			//
			// Answered **Unauthenticated** and not with roster's NotFound. The
			// difference is what a client does about it: told the row is
			// missing they retry the same call, and told they are not
			// authenticated they sign in again -- which is right, because the
			// thing that is gone is the person their session names. It is the
			// same distinction `auth.Resolver` asks for one layer down.
			return auth.Identity{}, status.Error(codes.Unauthenticated,
				"whoever this session names is no longer there")

		default:
			// Could not ask. The last answer stands; see the note above on why
			// this does not refuse.
			log.From(ctx).WarnContext(ctx, "could not confirm somebody with roster",
				slog.String("holder.id", id.Id),
				slog.String("why", err.Error()))

			return id, nil
		}
	})
}

// fresh reports whether this person was confirmed recently enough.
//
// Keyed by the person and not by the session, so somebody with four browsers
// open costs one call rather than four.
func (f *Fresh) fresh(who string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	at, ok := f.seen[who]

	return ok && time.Since(at) < f.Every
}

func (f *Fresh) ask(ctx context.Context, who string) error {
	k, err := pdid.Parse(who)
	if err != nil {
		return status.Errorf(codes.NotFound, "%s", err)
	}

	_, err = rstr.NewHolderServiceClient(f.roster.conn).Get(f.roster.as.Provide(ctx),
		rstr.HolderGetRequest_builder{
			Ref: rstr.HolderRef_builder{Id: k.Bytes()}.Build(),
		}.Build())
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.seen[who] = time.Now()

	return nil
}

// Forget drops what is remembered about somebody, so the next request asks
// again. It is what a deployment calls when it has heard from somewhere else
// that they are gone.
func (f *Fresh) Forget(who string) {
	if f == nil {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.seen, who)
}
