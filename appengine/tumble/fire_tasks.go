// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

func fireTasks(c context.Context, shards map[uint64]struct{}) bool {
	if len(shards) == 0 {
		return true
	}

	tq := taskqueue.Get(c)

	cfg := GetConfig(c)

	trf := cfg.TemporalRoundFactor
	eta := clock.Now(c).UTC().Add(cfg.TemporalMinDelay + trf).Round(trf)
	tasks := make([]*taskqueue.Task, 0, len(shards))

	for shard := range shards {
		tasks = append(tasks, &taskqueue.Task{
			Name: fmt.Sprintf("%d_%d", eta.Unix(), shard),

			Path: cfg.ProcessURL(eta, shard),

			ETA: eta,

			// TODO(riannucci): Tune RetryOptions?
		})
	}

	err := tq.AddMulti(tasks, cfg.Name)
	if err != nil {
		if merr, ok := err.(errors.MultiError); ok {
			lme := errors.NewLazyMultiError(len(merr))
			for i, err := range merr {
				if err == taskqueue.ErrTaskAlreadyAdded {
					continue
				}
				lme.Assign(i, err)
			}
			err = lme.Get()
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
	shards := make(map[uint64]struct{}, num)
	for i := uint64(0); i < num; i++ {
		shards[i] = struct{}{}
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
