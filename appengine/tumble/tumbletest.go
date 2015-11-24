// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"
	"golang.org/x/net/context"
)

// Testing is a high-level testing object for testing applications that use
// tumble.
type Testing struct {
	Config Config
}

// Context generates a correctly configured context with:
//   * luci/gae/impl/memory
//   * luci/luci-go/common/clock/testclock
//   * luci/luci-go/common/logging/memlogger
//
// It also correctly configures the "tumble.Mutation" indexes and taskqueue
// named in this Testing config.
//
// Calling this method will render the tumble package unsuitable for production
// usage, since it disables some internal timeouts. DO NOT CALL THIS METHOD
// IN PRODUCTION.
func (t Testing) Context() context.Context {
	dustSettleTimeout = 0

	ctx := memory.Use(memlogger.Use(context.Background()))
	ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
	ctx = Use(ctx, t.Config)

	cfg := GetConfig(ctx)

	taskqueue.Get(ctx).Testable().CreateQueue(cfg.Name)

	ds := datastore.Get(ctx)
	ds.Testable().AddIndexes(&datastore.IndexDefinition{
		Kind: "tumble.Mutation",
		SortBy: []datastore.IndexColumn{
			{Property: "ExpandedShard"},
			{Property: "TargetRoot"},
		},
	})
	ds.Testable().Consistent(true)

	return ctx
}

// Iterate makes a single iteration of the tumble service worker, and returns
// the number of shards that were processed.
//
// It will skip all work items if the test clock hasn't advanced in time
// enough.
func (t Testing) Iterate(c context.Context) int {
	tq := taskqueue.Get(c)
	cfg := GetConfig(c)
	clk := clock.Get(c).(testclock.TestClock)

	ret := 0
	tsks := tq.Testable().GetScheduledTasks()[cfg.Name]
	for _, tsk := range tsks {
		if tsk.ETA.After(clk.Now()) {
			continue
		}
		toks := strings.Split(tsk.Path, "/")
		rec := httptest.NewRecorder()
		ProcessShardHandler(c, rec, &http.Request{
			Header: http.Header{"X-AppEngine-QueueName": []string{cfg.Name}},
		}, httprouter.Params{
			{Key: "shard_id", Value: toks[4]},
			{Key: "timestamp", Value: toks[6]},
		})
		if rec.Code != 200 {
			panic(fmt.Errorf("ProcessShardHandler returned !200: %d", rec.Code))
		}
		if err := tq.Delete(tsk, cfg.Name); err != nil {
			panic(fmt.Errorf("Deleting task failed: %s", err))
		}
		ret++
	}
	return ret
}

// FireAllTasks will force all tumble shards to run in the future.
func (t Testing) FireAllTasks(c context.Context) {
	rec := httptest.NewRecorder()
	FireAllTasksHandler(c, rec, &http.Request{
		Header: http.Header{"X-Appengine-Cron": []string{"true"}},
	}, nil)
	if rec.Code != 200 {
		panic(fmt.Errorf("ProcessShardHandler returned !200: %d", rec.Code))
	}
}

// AdvanceTime advances the test clock enough so that Iterate will be able to
// pick up tasks in the task queue.
func (t Testing) AdvanceTime(c context.Context) {
	clk := clock.Get(c).(testclock.TestClock)
	cfg := GetConfig(c)
	toAdd := cfg.TemporalMinDelay + cfg.TemporalRoundFactor + time.Second
	logging.Infof(c, "adding %s to %s", toAdd, clk.Now())
	clk.Add(toAdd)
}

// Drain will run a loop, advancing time and iterating through tumble mutations
// until tumble's queue is empty. It returns the total number of processed
// shards.
func (t Testing) Drain(c context.Context) int {
	ret := 0
	for {
		t.AdvanceTime(c)
		processed := t.Iterate(c)
		if processed == 0 {
			break
		}
		ret += processed
	}
	return ret
}
