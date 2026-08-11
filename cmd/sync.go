package cmd

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/lesomnus/otx/log"

	"github.com/lesomnus/payday/auth/authsession"
	"github.com/lesomnus/payday/pdid"
	"github.com/lesomnus/payday/spin"

	"github.com/lesomnus/custody/internal/ent"

	rstr "github.com/lesomnus/roster/rstr"
)

// Hearing about somebody who left.
//
// # Why this exists at all
//
// custody's `Holder` rows are anchors, and the argument for them is that they
// carry **nothing that can go stale**: an identifier, which never changes, and
// a tenant, which does not either. That argument holds for every fact about a
// person except one.
//
// Somebody leaves. Their row in roster is erased, and custody hears about it
// when their next sign-in fails -- which is never, if they do not try. A session
// issued an hour ago runs for the rest of its twelve hours, and the anchor sits
// there being a valid `keeper` for an asset.
//
// So one fact travels, and it is the only one: **gone**. Everything else stays
// a read against roster when there is a screen to draw.
//
// # A stream and not a poll
//
// Polling asks roster for every person it has, on a timer, to find the one that
// changed. A stream is the same query run once and then kept, which is what
// payday's `Watch` is; the outbox behind it is what makes the answer survive
// this process restarting at the wrong moment.
//
// What it costs is a connection that has to be re-opened, and that is the whole
// of the loop below: a `Watch` that ends is a `Watch` that is started again,
// because the alternative is a deployment that stops hearing and says nothing.
//
// # Three things that had to be right at once
//
// Each of them silently produced a stream that opened, stayed open, and never
// said anything -- which is the failure mode a sync channel has, and why the
// test asserts a session ending rather than a message arriving.
//
//   - **The snapshot has to be taken.** A watch will not report a row
//     disappearing to a subscriber it never reported the row to: "a row that
//     never matched is not news". With `skip_snapshot` an erase of somebody
//     anchored months ago is dropped, and skipping it looks like the obvious
//     saving.
//   - **The signal is the value being absent, not the action.** An action is
//     the full name of the RPC that wrote -- `/roster.HolderService/Erase` --
//     so a check for the string "erase" never matches. It is also the wrong
//     thing to look at: a row leaves a subscriber's view by being erased, by
//     moving out of their tenants, or by no longer matching, and all three mean
//     the same thing here.
//   - **The write has to go over the wire.** A change is published by the gRPC
//     interceptor after the call answers, so an erase made in process through a
//     server instance publishes nothing. That is a property of `Ungated` worth
//     knowing generally: it is the door a deployment does its own work through,
//     and nothing watching hears any of it.
//
// # A gap that is left
//
// A filter naming somebody **already** erased is refused: the snapshot runs the
// same `List` a caller would, and a ref to a gone row is `NotFound` -- so a
// stream re-opened after a departure fails for everybody and keeps failing
// until the next `Refresh` drops that person from the filters. custody rebuilds
// the filters from its own anchors, which still hold the erased person, so this
// does not resolve on its own.
//
// Finishing it means reconciling: on `NotFound`, ask which of the anchored are
// still there and treat the rest as gone. That is the same answer this stream
// gives, arrived at the slow way, and it is what makes a restart safe.
//
// # What it does with one
//
// Ends their sessions. Not the anchor: the row is what a trail and an asset
// point at, and deleting it would either fail on the reference or take the
// history with it. Somebody who left is somebody who cannot sign in and whose
// name still appears on what they were responsible for, which is the honest
// state.

// Erased is what a session store has to be able to do for this to mean
// anything: end every session of one person, now.
//
// It is an interface rather than a concrete store because payday's
// `authsession.Store` deliberately keys by session and not by person -- a
// lookup that has to be fast on every request, and one that would need an index
// nobody else uses. So a deployment that wants this supplies it, and one that
// does not still gets the anchor marked.
type Erased interface {
	EndAll(ctx context.Context, who string) error
}

// Sync watches roster for people who left.
//
// It is a [spin.Spinner], so a deployment that has one runs it beside the
// server and one that does not writes nothing at all -- `spin.Run` finds what
// it was handed.
type Sync struct {
	roster   *Roster
	sessions Erased
	db       *ent.Client

	// Every is how long to wait before opening the stream again after it ends.
	// A stream ends for ordinary reasons -- a deploy on the other side, a
	// balancer's idle timeout -- so this is a pause and not a backoff.
	Every time.Duration

	// Refresh is how long one stream lasts before it is opened again over the
	// people custody has anchored **since**. See [Sync.watch].
	Refresh time.Duration
}

func NewSync(r *Roster, db *ent.Client, sessions Erased) *Sync {
	return &Sync{roster: r, sessions: sessions, db: db, Every: 5 * time.Second, Refresh: time.Minute}
}

// anchored is the people custody has a row for, as filters.
//
// Read from custody's own database rather than asked of roster: what this app
// cares about is who it has anchored, and roster's answer would be every person
// of every tenant it serves.
func (s *Sync) anchored(ctx context.Context) ([]*rstr.HolderFilter, error) {
	vs, err := s.db.Holder.Query().IDs(ctx)
	if err != nil {
		return nil, err
	}

	fs := make([]*rstr.HolderFilter, 0, len(vs))
	for _, v := range vs {
		fs = append(fs, rstr.HolderFilter_builder{
			Ref: rstr.HolderRef_builder{Id: v[:]}.Build(),
		}.Build())
	}

	return fs, nil
}

func (Sync) SpinName() string { return "roster" }

var _ spin.Spinner = (*Sync)(nil)
var _ spin.Named = (*Sync)(nil)

// Spin keeps a watch open for as long as the process runs.
func (s *Sync) Spin(ctx context.Context) error {
	if s.roster == nil {
		return nil
	}

	for {
		if err := s.watch(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			// Reported and not returned. A `Spin` that gave up would be a
			// deployment that stops hearing about departures and says so once,
			// in a line nobody is reading an hour later.
			log.From(ctx).WarnContext(ctx, "roster watch ended",
				slog.String("why", err.Error()))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.Every):
		}
	}
}

// watch opens one stream over the people custody has anchored, and reads it
// until it ends.
//
// # It names them
//
// payday refuses a watch with no filters, and the refusal is right: a watch
// runs its filters again for every write that touches the entity, so one with
// none is the whole table forever. What custody wants is not the whole table
// anyway -- it is the people it has a row for, and somebody it has never seen
// leaving is not news here.
//
// # And re-reads them
//
// The set changes: `OnDemand` anchors somebody the first time they arrive, and
// a stream opened before that does not carry them. So the stream is given a
// deadline and the loop re-reads -- a minute of a new person's session
// outliving them, against a design where a stream is re-opened on every
// anchor.
func (s *Sync) watch(ctx context.Context) error {
	who, err := s.anchored(ctx)
	if err != nil {
		return err
	}
	if len(who) == 0 {
		return nil
	}

	ctx, stop := context.WithTimeout(ctx, s.Refresh)
	defer stop()

	// The snapshot is **taken**, and skipping it does not work here -- which is
	// worth writing down, because it looks like the obvious saving.
	//
	// A watch will not report a row disappearing to a subscriber it never
	// reported the row to: `a row that never matched is not news`, and with the
	// snapshot skipped no row has ever matched. So an erase of somebody custody
	// anchored months ago would be silently dropped, which is the only event
	// this stream exists for.
	//
	// It costs one message per anchored person per reconnect, and it is bounded
	// by the filters: custody names who it cares about, so this is not the
	// directory.
	stream, err := rstr.NewHolderServiceClient(s.roster.conn).Watch(
		s.roster.as.Provide(ctx),
		rstr.HolderWatchRequest_builder{Filters: who}.Build())
	if err != nil {
		return err
	}

	for {
		v, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}

			return err
		}

		for _, w := range v.GetItems() {
			s.gone(ctx, w)
		}
	}
}

// gone ends the sessions of somebody this stream can no longer see.
//
// **The value being absent** is the signal, and not the action. An action is
// the full name of the RPC that made the write -- `/roster.HolderService/Erase`
// -- which this checked for the string "erase" and therefore never matched.
// That was the bug, and the shorter name is also the wrong thing to look at: a
// row can leave a subscriber's view by being erased, by moving out of the
// tenants they may see, or by stopping to match their filter, and to custody
// all three mean the same thing.
//
// A row that is still there is not news. A rename, a new address, a changed
// department are roster's, and reacting to them here would build the replica
// this design exists to avoid.
func (s *Sync) gone(ctx context.Context, v *rstr.HolderWatchItem) {
	if v.GetValue() != nil {
		return
	}

	who, err := pdid.From(v.GetId())
	if err != nil {
		return
	}

	log.From(ctx).InfoContext(ctx, "roster erased somebody; ending their sessions",
		slog.String("holder.id", who.String()))

	if s.sessions == nil {
		return
	}
	if err := s.sessions.EndAll(ctx, who.String()); err != nil {
		log.From(ctx).ErrorContext(ctx, "could not end sessions",
			slog.String("holder.id", who.String()),
			slog.String("why", err.Error()))
	}
}

// Sessions is [authsession.MemStore] with the one operation this needs.
//
// It is here rather than in payday because it is a different index: a session
// store is asked "whose is this key" on every request, and "which keys are
// theirs" only when somebody leaves. A real store answers the second with a
// `WHERE holder = ?`; this one walks what it holds, which is right for a map in
// one process and is the same reason that store is right for one replica.
type Sessions struct {
	*authsession.MemStore
}

func NewSessions() *Sessions { return &Sessions{authsession.NewMemStore()} }

var _ Erased = (*Sessions)(nil)

func (s *Sessions) EndAll(ctx context.Context, who string) error {
	return s.MemStore.DelBy(ctx, func(v authsession.Session) bool { return v.Id == who })
}
