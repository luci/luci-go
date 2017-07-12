// Copyright 2015 The LUCI Authors.
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

package tumble

import (
	"fmt"
	"math"
	"strings"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	tq "github.com/luci/gae/service/taskqueue"
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

func fireTasks(c context.Context, cfg *Config, shards map[taskShard]struct{}, loop bool) bool {
	if len(shards) == 0 {
		return true
	}

	nextSlot := mkTimestamp(cfg, clock.Now(c).UTC())
	logging.Fields{
		"slot": nextSlot,
	}.Debugf(c, "got next slot")

	tasks := make([]*tq.Task, 0, len(shards))

	// Transform our namespace into a valid task queue task name.
	ns := info.GetNamespace(c)
	taskNS := nsToTaskName(ns)

	for shard := range shards {
		eta := nextSlot
		if cfg.DelayedMutations && shard.time > eta {
			eta = shard.time
		}

		// Generate our task name.
		//
		// Fold namespace into the task name, since task names must be unique across
		// all namespaces.
		taskName := fmt.Sprintf("%d_%s_%d", eta, taskNS, shard.shard)
		if !loop {
			// Differentiate non-loop (cron) tasks from loop (Mutation-scheduled)
			// tasks so we don't supplant a long-running task with a cron task due to
			// timing.
			taskName += "_single"
		}

		tsk := &tq.Task{
			Name: taskName,
			Path: processURL(eta, shard.shard, ns, loop),
			ETA:  eta.Unix(),

			// TODO(riannucci): Tune RetryOptions?
		}
		tasks = append(tasks, tsk)
		logging.Infof(c, "added task %q %s %s", tsk.Name, tsk.Path, tsk.ETA)
	}

	b := tq.Batcher{}
	if err := errors.Filter(b.Add(ds.WithoutTransaction(c), baseName, tasks...), tq.ErrTaskAlreadyAdded); err != nil {
		logging.Warningf(c, "attempted to fire tasks %v, but failed: %s", shards, err)
		return false
	}
	return true
}

// nsToTaskName flattens a namespace into a string that can be part of a valid
// task queue task name.
func nsToTaskName(v string) string {
	// Escape single underscores in the namespace name.
	v = strings.Replace(v, "_", "__", -1)

	// Replace any invalid task queue name characters with underscore.
	return strings.Map(func(r rune) rune {
		switch {
		case (r >= 'a' && r <= 'z'),
			(r >= 'A' && r <= 'Z'),
			r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, v)
}
