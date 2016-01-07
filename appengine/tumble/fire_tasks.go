// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
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
	trf := cfg.TemporalRoundFactor
	eta := t.UTC().Add(cfg.TemporalMinDelay + trf).Round(trf)
	return timestamp(eta.Unix())
}

type taskShard struct {
	shard uint64
	time  timestamp
}

func fireTasks(c context.Context, shards map[taskShard]struct{}) bool {
	if len(shards) == 0 {
		return true
	}

	tq := taskqueue.Get(c)

	cfg := GetConfig(c)
	nextSlot := mkTimestamp(&cfg, clock.Now(c).UTC())
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

			Path: cfg.ProcessURL(eta, shard.shard),

			ETA: eta.Unix(),

			// TODO(riannucci): Tune RetryOptions?
		}
		tasks = append(tasks, tsk)
		logging.Infof(c, "added task %q %s %s", tsk.Name, tsk.Path, tsk.ETA)
	}

	err := tq.AddMulti(tasks, cfg.Name)
	if err != nil {
		if merr, ok := err.(errors.MultiError); ok {
			me := errors.MultiError(nil)
			for _, err := range merr {
				if err == taskqueue.ErrTaskAlreadyAdded {
					continue
				}
				me = append(me, err)
			}
			if me != nil {
				err = me
			}
		}
		if err != nil {
			logging.Warningf(c, "attempted to fire tasks %v, but failed: %s", shards, err)
		}
	}
	return err == nil
}

// FireAllTasks fires off 1 task per shard to ensure that no tumble work
// languishes forever. This may not be needed in a constantly-loaded system with
// good tumble key distribution.
func FireAllTasks(c context.Context) error {
	num := GetConfig(c).NumShards
	shards := make(map[taskShard]struct{}, num)
	for i := uint64(0); i < num; i++ {
		shards[taskShard{i, minTS}] = struct{}{}
	}

	err := error(nil)
	if !fireTasks(c, shards) {
		err = errors.New("unable to fire all tasks")
	}

	return err
}

// FireAllTasksHandler is a http handler suitable for installation into
// a httprouter. It expects `logging` and `luci/gae` services to be installed
// into the context.
//
// FireAllTasksHandler verifies that it was called within an Appengine Cron
// request, and then invokes the FireAllTasks function.
func FireAllTasksHandler(c context.Context, rw http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if err := FireAllTasks(c); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "fire_all_tasks failed: %s", err)
	} else {
		rw.Write([]byte("ok"))
	}
}
