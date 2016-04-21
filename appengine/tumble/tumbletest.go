// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"
)

// Testing is a high-level testing object for testing applications that use
// tumble.
type Testing struct {
	Config
	Service
}

// NewTesting returns a new Testing instance that is configured to use the
// default tumble configuration.
func NewTesting() *Testing {
	t := Testing{Config: defaultConfig}
	t.DustSettleTimeout = 0
	return &t
}

func (t *Testing) installSettings(c context.Context) {
	if t.NumShards == 0 || t.NumGoroutines == 0 || t.ProcessMaxBatchSize == 0 {
		panic(errors.New("testing configuration is not valid... maybe call tumble.NewTesting()"))
	}

	settings.Set(c, baseName, &t.Config, "tumble.Testing", "for testing")
}

// Context generates a correctly configured context with:
//   * luci/gae/impl/memory
//   * luci/luci-go/common/clock/testclock
//   * luci/luci-go/common/logging/memlogger
//   * luci/luci-go/server/settings (MemoryStorage)
//
// It also correctly configures the "tumble.Mutation" indexes and taskqueue
// named in this Testing config.
//
// Calling this method will render the tumble package unsuitable for production
// usage, since it disables some internal timeouts. DO NOT CALL THIS METHOD
// IN PRODUCTION.
func (t *Testing) Context() context.Context {
	ctx := memory.Use(memlogger.Use(context.Background()))
	ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC.Round(time.Millisecond))
	ctx = settings.Use(ctx, settings.New(&settings.MemoryStorage{}))
	t.installSettings(ctx)

	taskqueue.Get(ctx).Testable().CreateQueue(baseName)

	ds := datastore.Get(ctx)
	ds.Testable().AddIndexes(&datastore.IndexDefinition{
		Kind: "tumble.Mutation",
		SortBy: []datastore.IndexColumn{
			{Property: "ExpandedShard"},
			{Property: "TargetRoot"},
		},
	})

	if t.Config.DelayedMutations {
		ds.Testable().AddIndexes(&datastore.IndexDefinition{
			Kind: "tumble.Mutation",
			SortBy: []datastore.IndexColumn{
				{Property: "TargetRoot"},
				{Property: "ProcessAfter"},
			},
		})
	}
	ds.Testable().Consistent(true)

	return ctx
}

// Iterate makes a single iteration of the tumble service worker, and returns
// the number of shards that were processed.
//
// It will skip all work items if the test clock hasn't advanced in time
// enough.
func (t *Testing) Iterate(c context.Context) int {
	t.installSettings(c)

	tq := taskqueue.Get(c)
	clk := clock.Get(c).(testclock.TestClock)

	ret := 0
	tsks := tq.Testable().GetScheduledTasks()[baseName]
	logging.Debugf(c, "got tasks: %v", tsks)
	for _, tsk := range tsks {
		logging.Debugf(c, "found task: %v", tsk)
		if tsk.ETA.After(clk.Now().UTC()) {
			logging.Infof(c, "skipping task: %s", tsk.Path)
			continue
		}
		toks := strings.Split(tsk.Path, "/")
		rec := httptest.NewRecorder()
		t.ProcessShardHandler(c, rec, &http.Request{
			Header: http.Header{"X-AppEngine-QueueName": []string{baseName}},
		}, httprouter.Params{
			{Key: "shard_id", Value: toks[4]},
			{Key: "timestamp", Value: toks[6]},
		})
		if rec.Code != 200 {
			lmsg := logging.Get(c).(*memlogger.MemLogger).Messages()
			panic(fmt.Errorf("ProcessShardHandler returned !200: %d: %#v", rec.Code, lmsg))
		}
		if err := tq.Delete(tsk, baseName); err != nil {
			panic(fmt.Errorf("Deleting task failed: %s", err))
		}
		ret++
	}
	return ret
}

// FireAllTasks will force all tumble shards to run in the future.
func (t *Testing) FireAllTasks(c context.Context) {
	t.installSettings(c)

	rec := httptest.NewRecorder()
	t.FireAllTasksHandler(c, rec, &http.Request{
		Header: http.Header{"X-Appengine-Cron": []string{"true"}},
	}, nil)
	if rec.Code != 200 {
		panic(fmt.Errorf("ProcessShardHandler returned !200: %d", rec.Code))
	}
}

// AdvanceTime advances the test clock enough so that Iterate will be able to
// pick up tasks in the task queue.
func (t *Testing) AdvanceTime(c context.Context) {
	t.installSettings(c)

	clk := clock.Get(c).(testclock.TestClock)
	toAdd := time.Duration(t.TemporalMinDelay) + time.Duration(t.TemporalRoundFactor) + time.Second
	logging.Infof(c, "adding %s to %s", toAdd, clk.Now().UTC())
	clk.Add(toAdd)
}

// Drain will run a loop, advancing time and iterating through tumble mutations
// until tumble's queue is empty. It returns the total number of processed
// shards.
func (t *Testing) Drain(c context.Context) int {
	t.installSettings(c)

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

// ResetLog resets the current memory logger to the empty state.
func (t *Testing) ResetLog(c context.Context) {
	logging.Get(c).(*memlogger.MemLogger).Reset()
}

// DumpLog dumps the current memory logger to stdout to help with debugging.
func (t *Testing) DumpLog(c context.Context) {
	logging.Get(c).(*memlogger.MemLogger).Dump(os.Stdout)
}
