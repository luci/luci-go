// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"fmt"
	"math"
	"time"

	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

type timestamp int64

const minTS timestamp = math.MinInt64

func (t timestamp) Unix() time.Time {
	return time.Unix((int64)(t), 0).UTC()
}

func mkTimestamp(cfg *Config, t time.Time) timestamp {
	trf := time.Duration(cfg.TemporalRoundFactor)
	eta := t.UTC().Add(time.Duration(cfg.TemporalMinDelay) + trf).Round(trf)
	return timestamp(eta.Unix())
}

type taskShard struct {
	shard uint64
	time  timestamp
}

func fireTasks(c context.Context, cfg *Config, shards map[taskShard]struct{}) bool {
	if len(shards) == 0 {
		return true
	}

	// Tumble uses the empty namespace for all task queue tasks.
	c = info.Get(c).MustNamespace("")
	tq := taskqueue.Get(c)

	nextSlot := mkTimestamp(cfg, clock.Now(c).UTC())
	logging.Fields{
		"slot": nextSlot,
	}.Debugf(c, "got next slot")

	tasks := make([]*taskqueue.Task, 0, len(shards))

	for shard := range shards {
		eta := nextSlot
		if cfg.DelayedMutations && shard.time > eta {
			eta = shard.time
		}
		tsk := &taskqueue.Task{
			Name: fmt.Sprintf("%d_%d", eta, shard.shard),

			Path: processURL(eta, shard.shard),

			ETA: eta.Unix(),

			// TODO(riannucci): Tune RetryOptions?
		}
		tasks = append(tasks, tsk)
		logging.Infof(c, "added task %q %s %s", tsk.Name, tsk.Path, tsk.ETA)
	}

	if err := errors.Filter(tq.AddMulti(tasks, baseName), taskqueue.ErrTaskAlreadyAdded); err != nil {
		logging.Warningf(c, "attempted to fire tasks %v, but failed: %s", shards, err)
		return false
	}
	return true
}
