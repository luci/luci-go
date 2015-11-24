// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"container/list"
	"math"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// WalkAttemptNumWorkers is the number of workers that WalkAttempt will use
// simulataneously.
const WalkAttemptNumWorkers = 8

// AttemptOpts are settings for WalkAttempts that describe which data will be
// loaded while walking the attempt.
type AttemptOpts struct {
	// DFS sets whether this is a Depth-first (ish) or a Breadth-first (ish) load.
	// Since the load operation is multi-threaded, the search order is best
	// effort, but will actually be some hybrid between DFS and BFS. This setting
	// controls the bias dierection of the hybrid loading algorithm.
	DFS      bool
	BackDeps bool
	FwdDeps  bool
	Result   bool
	MaxDepth uint32

	testSimulateTimeout bool
}

type attemptWorkItm struct {
	aid   *types.AttemptID
	depth uint32

	data *display.Data
	err  error
}

func loadDeps(ds datastore.Interface, fwd bool, from *types.AttemptID) (*display.DepsFromAttempt, error) {
	qStr := "FwdDep"
	fromKey := ds.MakeKey("Attempt", from.ID())
	if !fwd {
		qStr = "BackDep"
		fromKey = ds.MakeKey("BackDepGroup", from.ID())
	}

	ret := &display.DepsFromAttempt{From: *from}

	q := datastore.NewQuery(qStr).Ancestor(fromKey)
	err := ds.Run(q, func(k *datastore.Key, _ datastore.CursorCB) bool {
		toAID := types.NewAttemptID(k.StringID())
		ret.To.Merge(&display.QuestAttempts{
			QuestID:  toAID.QuestID,
			Attempts: types.U32s{toAID.AttemptNum},
		})
		return true
	})

	if len(ret.To) == 0 {
		ret = nil
	}

	return ret, err
}

func walkAttemptWorkerInner(ds datastore.Interface, opts *AttemptOpts, aid *types.AttemptID) (ret *display.Data, err error) {
	atmpt := &model.Attempt{AttemptID: *aid}
	if err = ds.Get(atmpt); err != nil {
		if err == datastore.ErrNoSuchEntity {
			err = nil
		}
		return
	}

	ret = &display.Data{}
	ret.Attempts.Merge(atmpt.ToDisplay())

	// TODO(riannucci): allow backwards graph traversal
	deps, err := loadDeps(ds, true, aid)
	if err != nil {
		return
	}
	ret.FwdDeps.Merge(deps)

	if opts.BackDeps {
		deps, err = loadDeps(ds, false, aid)
		if err != nil {
			return
		}
		ret.BackDeps.Merge(deps)
	}

	if opts.Result && atmpt.State == types.Finished {
		ar := &model.AttemptResult{Attempt: ds.KeyForObj(atmpt)}
		err = ds.Get(ar)
		if err != nil {
			return
		}
		ret.AttemptResults.Merge(ar.ToDisplay())
	}

	return
}

func walkAttemptWorker(c context.Context, opts *AttemptOpts, workChan <-chan *attemptWorkItm, rsltChan chan<- *attemptWorkItm) {
	ds := datastore.Get(c)
	for itm := range workChan {
		if err := c.Err(); err != nil || opts.testSimulateTimeout {
			return
		}
		itm.data, itm.err = walkAttemptWorkerInner(ds, opts, itm.aid)
		rsltChan <- itm
	}
}

func walkAttemptMainLoop(c context.Context, opts *AttemptOpts, start types.AttemptID, workChan chan<- *attemptWorkItm, rsltChan <-chan *attemptWorkItm) *display.Data {
	ret := &display.Data{}

	stop := (func())(nil)
	if opts.testSimulateTimeout {
		c, stop = context.WithCancel(c)
	}

	queue := list.New()
	add := queue.PushBack
	if opts.DFS {
		add = queue.PushFront
	}

	get := func() *attemptWorkItm {
		e := queue.Front()
		if e == nil {
			return nil
		}
		return queue.Remove(e).(*attemptWorkItm)
	}

	add(&attemptWorkItm{aid: &start})

	outstandingWorkers := 0
	for {
		if err := c.Err(); err != nil {
			ret.Timeout = true
			return ret
		}

		if opts.testSimulateTimeout {
			stop()
		}

		for i := 0; i < (WalkAttemptNumWorkers - outstandingWorkers); i++ {
			itm := get()
			if itm == nil {
				break
			}
			workChan <- itm
			outstandingWorkers++
		}
		if outstandingWorkers == 0 && queue.Len() == 0 {
			return ret
		}

		select {
		case itm := <-rsltChan:
			outstandingWorkers--

			if itm.err != nil {
				ret.HadErrors = true
				logging.Errorf(c, "err: %s", itm.err)
			}

			merged := ret.Merge(itm.data)
			if merged != nil && (opts.MaxDepth == math.MaxUint32 || itm.depth < opts.MaxDepth) {
				for _, fd := range merged.FwdDeps {
					for _, to := range fd.To {
						for _, atmpt := range to.Attempts {
							newItm := &attemptWorkItm{aid: &types.AttemptID{
								QuestID: to.QuestID, AttemptNum: atmpt,
							}, depth: itm.depth + 1}
							add(newItm)
						}
					}
				}
			}

		case <-c.Done():
		}
	}
}

// WalkAttempts does a graph traversal of the forward deps from the provided
// Attempt. It will execute until the context is cancelled, MaxDepth is reached,
// or the graph is fully traversed, whichever comes first.
//
// This traversal is eventually consistent, however if the starting Attempt has
// been in the Finished state for any appreciable amount of time, it is VERY
// LIKELY that the graph will be immutable after that point. The only possible
// modification would be a slow index adding a new Finished Attempt as a
// dependency, or an AttemptResult expiring.
func WalkAttempts(c context.Context, start types.AttemptID, opts *AttemptOpts) *display.Data {
	c, stop := context.WithCancel(c)
	defer stop()

	if opts == nil {
		opts = &AttemptOpts{}
	}

	workChan := make(chan *attemptWorkItm, WalkAttemptNumWorkers)
	defer close(workChan)

	rsltChan := make(chan *attemptWorkItm, WalkAttemptNumWorkers)
	defer close(rsltChan)

	for i := 0; i < WalkAttemptNumWorkers; i++ {
		go walkAttemptWorker(c, opts, workChan, rsltChan)
	}

	ret := walkAttemptMainLoop(c, opts, start, workChan, rsltChan)

	if !opts.FwdDeps {
		ret.FwdDeps = nil
	}

	return ret
}
