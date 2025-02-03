// Copyright 2024 The LUCI Authors.
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

package scan

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/dsmapper/dsmapperlite"

	"go.chromium.org/luci/swarming/server/model"
)

// BotVisitor examines bots in parallel.
//
// Should keep track of errors internally during the scan, reporting them only
// in the end in Finalize.
type BotVisitor interface {
	// ID returns an unique identifier of this visitor used for storing its state.
	ID() string

	// Frequency returns how frequently this visitor should run.
	//
	// This is approximate. Granularity would be ~= 1 min. Value of 0 means this
	// visitor will approximately run every minute.
	Frequency() time.Duration

	// Prepare prepares the visitor to use `shards` parallel queries.
	//
	// It should initialize per-shard state used by Visit. `lastRun` is the last
	// time this visitor ran or a zero time if this is the first time it is
	// running.
	Prepare(ctx context.Context, shards int, lastRun time.Time)

	// Visit is called for every bot.
	//
	// The overall scan is split into multiple shards. Within a shard the scan is
	// sequential, but shards themselves are processed in parallel.
	Visit(ctx context.Context, shard int, bot *model.BotInfo)

	// Finalize is called once the scan is done.
	//
	// It is passed an error if the scan was incomplete. If the scan was complete,
	// (i.e. Visit visited all bots), it receives nil.
	//
	// The returned error will be reported as an overall scan error.
	Finalize(ctx context.Context, scanErr error) error
}

// Bots visits all bots via multiple parallel queries.
//
// Takes a list of all registered visitors. Will run only ones that are due for
// execution now per their run frequency.
//
// Returns a multi-error with the scan error (if any) and errors from all
// visitors' finalizers (if any) in arbitrary order.
func Bots(ctx context.Context, visitors []BotVisitor) error {
	const shardCount = 128

	state, err := fetchState(ctx)
	if err != nil {
		return err
	}

	startTS := clock.Now(ctx).UTC()
	var total atomic.Int32

	visitors = visitorsToRun(ctx, state, visitors, startTS)
	if len(visitors) == 0 {
		return nil
	}

	for _, v := range visitors {
		v.Prepare(ctx, shardCount, state.lastScan(v.ID()))
	}

	scanErr := dsmapperlite.Map(ctx, model.BotInfoQuery(), shardCount, 1200,
		func(_ context.Context, shardIdx int, bot *model.BotInfo) error {
			// These appear to be phantom GCE provider bots which are either being
			// created or weren't fully deleted. They don't have `state` JSON dict
			// populated, and they aren't really running.
			if !bot.LastSeen.IsSet() || len(bot.State.JSON) == 0 {
				return nil
			}
			// Log broken state in a central place. Visitors usually just ignore it.
			if err := bot.State.Unseal(); err != nil {
				logging.Warningf(ctx, "Bot %s: bad state:\n:%s", bot.BotID(), bot.State)
			}
			for _, v := range visitors {
				v.Visit(logging.SetField(ctx, "visitor", v.ID()), shardIdx, bot)
			}
			total.Add(1)
			return nil
		},
	)
	if scanErr != nil {
		logging.Errorf(ctx, "Scan failed after %s. Visited bots: %d", clock.Since(ctx, startTS), total.Load())
		scanErr = errors.Annotate(scanErr, "scanning BotInfo").Err()
	} else {
		logging.Infof(ctx, "Scan done in %s. Total visited bots: %d", clock.Since(ctx, startTS), total.Load())
	}

	// Run finalizers in parallel, they may do slow IO. Collect all errors.
	errs := make(chan error, len(visitors))
	for _, v := range visitors {
		v := v
		go func() {
			errs <- v.Finalize(logging.SetField(ctx, "visitor", v.ID()), scanErr)
		}()
	}
	var merr errors.MultiError
	merr.MaybeAdd(scanErr)
	for range visitors {
		merr.MaybeAdd(<-errs)
	}

	// Store the updated state.
	for _, v := range visitors {
		state.bumpLastScan(v.ID(), startTS)
	}
	merr.MaybeAdd(storeState(ctx, state))

	return merr.AsError()
}

type botScannerState map[string]*model.BotVisitorState

func (state botScannerState) lastScan(id string) time.Time {
	if s, ok := state[id]; ok {
		return s.LastScan
	}
	return time.Time{}
}

func (state botScannerState) bumpLastScan(id string, when time.Time) {
	if s, ok := state[id]; ok {
		s.LastScan = when
	} else {
		state[id] = &model.BotVisitorState{
			VisitorID: id,
			LastScan:  when,
		}
	}
}

func fetchState(ctx context.Context) (botScannerState, error) {
	ent := &model.BotScannerState{Key: model.BotScannerStateKey(ctx)}
	if err := datastore.Get(ctx, ent); err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
		return nil, errors.Annotate(err, "fetching BotScannerState").Err()
	}
	out := make(botScannerState, len(ent.VisitorState))
	for i := range ent.VisitorState {
		out[ent.VisitorState[i].VisitorID] = &ent.VisitorState[i]
	}
	return out, nil
}

func storeState(ctx context.Context, state botScannerState) error {
	ent := &model.BotScannerState{
		Key:          model.BotScannerStateKey(ctx),
		VisitorState: make([]model.BotVisitorState, 0, len(state)),
	}
	for _, v := range state {
		ent.VisitorState = append(ent.VisitorState, *v)
	}
	slices.SortFunc(ent.VisitorState, func(a, b model.BotVisitorState) int {
		return strings.Compare(a.VisitorID, b.VisitorID)
	})
	if err := datastore.Put(ctx, ent); err != nil {
		return errors.Annotate(err, "storing BotScannerState").Err()
	}
	return nil
}

func visitorsToRun(ctx context.Context, state botScannerState, all []BotVisitor, now time.Time) []BotVisitor {
	var visitors []BotVisitor
	for _, v := range all {
		if lastScan := state.lastScan(v.ID()); !lastScan.IsZero() {
			elapsed := now.Sub(lastScan)
			expected := lastScan.Add(v.Frequency())
			if !now.Before(expected) {
				logging.Infof(ctx, "Running %s [last run started %1.fs ago]", v.ID(), elapsed.Seconds())
				visitors = append(visitors, v)
			} else {
				logging.Infof(ctx, "Skipping %s [last run started %1.fs ago, next run is in %1.fs]",
					v.ID(), elapsed.Seconds(), expected.Sub(now).Seconds())
			}
		} else {
			logging.Infof(ctx, "Running %s for the first time", v.ID())
			visitors = append(visitors, v)
		}
	}
	return visitors
}
