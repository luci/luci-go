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
	"sync/atomic"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/dsmapper/dsmapperlite"

	"go.chromium.org/luci/swarming/server/model"
)

// BotVisitor examines bots in parallel.
//
// Should keep track of errors internally during the scan, reporting them only
// in the end in Finalize.
type BotVisitor interface {
	// Prepare prepares the visitor to use `shards` parallel queries.
	//
	// It should initialize per-shard state used by Visit.
	Prepare(ctx context.Context, shards int)

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
// Returns a multi-error with the scan error (if any) and errors from all
// visitors' finalizers (if any) in arbitrary order.
func Bots(ctx context.Context, visitors []BotVisitor) error {
	const shardCount = 128

	startTS := clock.Now(ctx)
	var total atomic.Int32

	for _, v := range visitors {
		v.Prepare(ctx, shardCount)
	}

	scanErr := dsmapperlite.Map(ctx, model.BotInfoQuery(), shardCount, 1200,
		func(ctx context.Context, shardIdx int, bot *model.BotInfo) error {
			// These appear to be phantom GCE provider bots which are either being
			// created or weren't fully deleted. They don't have `state` JSON dict
			// populated, and they aren't really running.
			if !bot.LastSeen.IsSet() || len(bot.State) == 0 {
				return nil
			}
			for _, v := range visitors {
				v.Visit(ctx, shardIdx, bot)
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
		go func() { errs <- v.Finalize(ctx, scanErr) }()
	}
	var merr errors.MultiError
	merr.MaybeAdd(scanErr)
	for range visitors {
		merr.MaybeAdd(<-errs)
	}
	return merr.AsError()
}
