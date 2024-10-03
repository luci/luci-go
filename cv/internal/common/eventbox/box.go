// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package eventbox

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox/dsset"
	"go.chromium.org/luci/cv/internal/tracing"
)

// Recipient is the recipient of the events.
type Recipient struct {
	// Key is the Datastore key of the recipient.
	//
	// The corresponding entity doesn't have to exist.
	Key *datastore.Key
	// MonitoringString is the value for the metric field "recipient".
	//
	// There should be very few distinct values.
	MonitoringString string
}

// Emit emits a new event with provided value and auto-generated unique ID.
func Emit(ctx context.Context, value []byte, to Recipient) error {
	// TombstonesDelay doesn't matter for Add.
	d := dsset.Set{Parent: to.Key}
	// Keep IDs well distributed, but record creation time in it.
	// See also oldestEventAge().
	id := fmt.Sprintf("%s/%d", uuid.New().String(), clock.Now(ctx).UnixNano())
	if err := d.Add(ctx, []dsset.Item{{ID: id, Value: value}}); err != nil {
		return errors.Annotate(err, "failed to send event").Err()
	}
	metricSent.Add(ctx, 1, to.MonitoringString)
	return nil
}

// TombstonesDelay is exposed to mitigate frequent errors in CV e2e tests when
// tasks are run in parallel with fake clock.
var TombstonesDelay = 5 * time.Minute

// List returns unprocessed events. For use in tests only.
func List(ctx context.Context, r Recipient) (Events, error) {
	d := dsset.Set{
		Parent:          r.Key,
		TombstonesDelay: TombstonesDelay,
	}
	const effectivelyUnlimited = 1000000
	switch l, err := d.List(ctx, effectivelyUnlimited); {
	case err != nil:
		return nil, err
	case len(l.Items) == effectivelyUnlimited:
		return nil, fmt.Errorf("fetched possibly not all events (limit: %d)", effectivelyUnlimited)
	default:
		return toEvents(l.Items), nil
	}
}

// ProcessBatch reliably processes outstanding events, while transactionally modifying state
// and performing arbitrary side effects.
//
// Returns:
//   - a slice of non-nil post process functions which SHOULD be executed
//     immediately after calling this function. Those are generally extra work
//     that needs to be done as the result of state modification.
//   - error while processing events. Tags the error with common.DSContentionTag
//     if entity's EVersion has changed or there is contention on Datastore
//     entities involved in a transaction.
func ProcessBatch(ctx context.Context, r Recipient, p Processor, maxEvents int) (_ []PostProcessFn, err error) {
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/cv/internal/eventbox/ProcessBatch",
		attribute.String("recipient", r.MonitoringString),
	)
	defer func() { tracing.End(span, err) }()
	postProcessFn, err := processBatch(ctx, r, p, maxEvents)
	if common.IsDatastoreContention(err) {
		err = common.DSContentionTag.Apply(err)
	}
	return postProcessFn, err
}

func processBatch(ctx context.Context, r Recipient, p Processor, maxEvents int) ([]PostProcessFn, error) {
	var state State
	var expectedEV EVersion
	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		state, expectedEV, err = p.LoadState(ectx)
		return
	})
	d := dsset.Set{
		Parent:          r.Key,
		TombstonesDelay: TombstonesDelay,
	}
	var listing *dsset.Listing
	eg.Go(func() (err error) {
		listing, err = listAndCleanup(ectx, r, &d, maxEvents)
		return
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// Compute resulting state before transaction.
	transitions, garbage, err := p.PrepareMutation(ctx, toEvents(listing.Items), state)
	if gErr := deleteSemanticGarbage(ctx, r, &d, garbage); gErr != nil {
		return nil, gErr
	}
	if err != nil {
		return nil, err
	}
	transitions = withoutNoops(transitions, state)
	if len(transitions) == 0 {
		return nil, nil // nothing to do.
	}

	var innerErr error
	var postProcessFns []PostProcessFn
	var eventsRemoved int
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		//  reset, since this func can be retried
		postProcessFns = nil
		eventsRemoved = 0

		switch latestEV, err := p.FetchEVersion(ctx); {
		case err != nil:
			return err
		case latestEV != expectedEV:
			return errors.Reason(
				"Datastore contention: EVersion read %d, but expected %d", latestEV, expectedEV,
			).Tag(transient.Tag).Tag(common.DSContentionTag).Err()
		}

		popOp, err := d.BeginPop(ctx, listing)
		if err != nil {
			return errors.Annotate(err, "failed to BeginPop").Err()
		}

		var newState State
		for _, t := range transitions {
			if err := t.apply(ctx, popOp); err != nil {
				return err
			}
			newState = t.TransitionTo
			if t.PostProcessFn != nil {
				postProcessFns = append(postProcessFns, t.PostProcessFn)
			}
			eventsRemoved += len(t.Events)
		}

		if newState != state {
			if err := p.SaveState(ctx, newState, expectedEV+1); err != nil {
				return err
			}
		}
		return dsset.FinishPop(ctx, popOp)
	}, nil)

	switch {
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to commit mutation").Tag(transient.Tag).Err()
	default:
		metricRemoved.Add(ctx, int64(eventsRemoved), r.MonitoringString)
		return postProcessFns, nil
	}
}

// Processor defines safe way to process events in a batch.
type Processor interface {
	// LoadState is called to load the state before a transaction.
	LoadState(context.Context) (State, EVersion, error)
	// PrepareMutation is called before a transaction to compute transitions based
	// on a batch of events.
	//
	// The events in a batch are an arbitrary subset of all outstanding events.
	// Because loading of events isn't synchronized with event senders,
	// a recipient of events may see them in different order than the origination
	// order, even if events were produced by a single sender.
	//
	// All actions that must be done atomically with updating state must be
	// encapsulated inside Transition.SideEffectFn callback.
	//
	// Garbage events will be deleted non-transactionally before executing
	// transactional transitions. These events may still be processed by a
	// concurrent invocation of a Processor. The garbage events slice may re-use
	// the given events slice. The garbage will be deleted even if PrepareMutation returns
	// non-nil error.
	//
	// For correctness, two concurrent invocation of a Processor must choose the
	// same events to be deleted as garbage. Consider scenario of 2 events A and B
	// deemed semantically the same and 2 concurrent Processor invocations:
	//   P1: let me delete A and hope to transactionally process B.
	//   P2:  ............ B and ............................... A.
	// Then, it's a real possibility that A and B are both deleted AND no neither
	// P1 nor P2 commits a transaction, thus forever forgetting about A and B.
	PrepareMutation(context.Context, Events, State) (transitions []Transition, garbage Events, err error)
	// FetchEVersion is called at the beginning of a transaction.
	//
	// The returned EVersion is compared against the one associated with a state
	// loaded via GetState. If different, the transaction is aborted and new state
	// isn't saved.
	FetchEVersion(ctx context.Context) (EVersion, error)
	// SaveState is called in a transaction to save the state if it has changed.
	//
	// The passed eversion is incremented value of eversion of what GetState
	// returned before.
	SaveState(context.Context, State, EVersion) error
}

// Event is an incoming event.
type Event dsset.Item

// Events are incoming events.
type Events []Event

// toEvents is an annoying redundant malloc to avoid exposing dsset.Item :(
func toEvents(items []dsset.Item) Events {
	es := make(Events, len(items))
	for i, item := range items {
		es[i] = Event(item)
	}
	return es
}

func listAndCleanup(ctx context.Context, r Recipient, d *dsset.Set, maxEvents int) (*dsset.Listing, error) {
	tStart := clock.Now(ctx)
	listing, err := d.List(ctx, maxEvents)
	metricListDurationsS.Add(ctx, float64(clock.Since(ctx, tStart).Milliseconds()), r.MonitoringString, monitoringResult(err))
	if err != nil {
		return nil, err
	}
	metricSize.Set(ctx, int64(len(listing.Items)), r.MonitoringString)
	metricOldestAgeS.Set(ctx, oldestEventAge(ctx, listing.Items).Seconds(), r.MonitoringString)

	if err := dsset.CleanupGarbage(ctx, listing.Garbage); err != nil {
		return nil, err
	}
	metricRemoved.Add(ctx, int64(len(listing.Garbage)), r.MonitoringString)
	return listing, nil
}

func oldestEventAge(ctx context.Context, items []dsset.Item) time.Duration {
	var oldest time.Time
	for _, item := range items {
		// NOTE: there can be some events with old IDs, which didn't record
		// timestamps.
		if parts := strings.SplitN(item.ID, "/", 2); len(parts) == 2 {
			if unixNano, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				if t := time.Unix(0, unixNano); oldest.IsZero() || oldest.After(t) {
					oldest = t
				}
			}
		}
	}
	if oldest.IsZero() {
		return 0
	}
	age := clock.Since(ctx, oldest)
	if age < 0 {
		// Clocks aren't perfectly synchronized, so round age up to 0.
		age = 0
	}
	return age
}

func deleteSemanticGarbage(ctx context.Context, r Recipient, d *dsset.Set, events Events) error {
	l := len(events)
	if l == 0 {
		return nil
	}
	logging.Debugf(ctx, "eventbox deleting %d semantic garbage events before transaction", l)
	i := -1
	err := d.Delete(ctx, func() string {
		i++
		if i < l {
			return events[i].ID
		}
		return ""
	})
	if err != nil {
		return errors.Annotate(err, "failed to delete %d semantic garbage events before transaction", l).Err()
	}
	metricRemoved.Add(ctx, int64(l), r.MonitoringString)
	return nil
}

// State is an arbitrary object.
//
// Use a pointer to an actual state.
type State any

// EVersion is recipient entity version.
type EVersion int64

// PostProcessFn should be executed after event processing completes.
type PostProcessFn func(context.Context) error

// SideEffectFn performs side effects with a Datastore transaction context.
// See Transition.SideEffectFn doc.
type SideEffectFn func(context.Context) error

// Chain combines several SideEffectFn.
//
// NOTE: modifies incoming ... slice.
func Chain(fs ...SideEffectFn) SideEffectFn {
	nonNil := fs[:0]
	for _, f := range fs {
		if f != nil {
			nonNil = append(nonNil, f)
		}
	}
	if len(nonNil) == 0 {
		return nil
	}
	return func(ctx context.Context) error {
		for _, f := range nonNil {
			if err := f(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

// Transition is a state transition.
type Transition struct {
	// SideEffectFn is called in a transaction to atomically with the state change
	// execute any side effects of a state transition.
	//
	// Typical use is notifying other CV components via TQ tasks.
	// Can be nil, meaning there no side effects to execute.
	//
	// TODO(tandrii): introduce error tag to indicate that failure was clean and
	// should be treated as if Transition wasn't started, s.t. progress of all
	// transitions before can be saved.
	SideEffectFn SideEffectFn
	// Events to consume with this transition.
	Events Events
	// TransitionTo is a state to transition to.
	//
	// It's allowed to transition to the exact same state.
	TransitionTo State
	// PostProcessFn is the function to be called by the eventbox user after
	// event processing completes.
	//
	// Note that it will be called outside of the transaction of all state
	// transitions, so the operation inside this function is not expected
	// to be atomic with this state transition.
	PostProcessFn PostProcessFn
}

func (t *Transition) apply(ctx context.Context, p *dsset.PopOp) error {
	if t.SideEffectFn != nil {
		if err := t.SideEffectFn(ctx); err != nil {
			return err
		}
	}
	for _, e := range t.Events {
		_ = p.Pop(e.ID) // Silently ignore if event has already been consumed.
	}
	return nil
}

// isNoop returns true if the Transition can be skipped entirely.
func (t *Transition) isNoop(oldState State) bool {
	return t.SideEffectFn == nil && len(t.Events) == 0 && t.TransitionTo == oldState && t.PostProcessFn == nil
}

// withoutNoops returns only actionable transitions in the original order.
//
// Modifies incoming slice.
func withoutNoops(all []Transition, s State) []Transition {
	ret := all[:0]
	for _, t := range all {
		if t.isNoop(s) {
			continue
		}
		ret = append(ret, t)
		s = t.TransitionTo
	}
	return ret
}
