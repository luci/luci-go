// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/enums/attempt"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"golang.org/x/net/context"
)

// AckFwdDep records the fact that a BackDep was successfully created for this
// FwdDep. It may also propagate Finished information (e.g. that the depended-on
// Attempt was actually already completed at the time that the dependency was
// taken on it).
//
// AckFwdDep is also used to propagate the fact that an Attempt (A) completed
// back to another Attempt (B) that's blocked on A.
type AckFwdDep struct {
	Dep           *model.FwdEdge
	DepIsFinished bool
}

// Root implements tumble.Mutation.
func (f *AckFwdDep) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).MakeKey("Attempt", f.Dep.From.ID())
}

// RollForward implements tumble.Mutation.
func (f *AckFwdDep) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	ds := datastore.Get(c)

	atmpt, fdep := f.Dep.Fwd(c)
	err = ds.GetMulti([]interface{}{atmpt, fdep})
	if err != nil {
		return
	}

	// if the attempt and fdep aren't on the same execution, then bail
	if atmpt.CurExecution != fdep.ForExecution {
		return
	}

	needPut := false

	idx := uint64(fdep.BitIndex)

	if !atmpt.AddingDepsBitmap.IsSet(idx) {
		err = atmpt.AddingDepsBitmap.Set(idx)
		impossible(err)

		if atmpt.AddingDepsBitmap.All(true) {
			atmpt.State.MustEvolve(attempt.Blocked)
		}

		needPut = true
	}

	if f.DepIsFinished {
		if !atmpt.WaitingDepBitmap.IsSet(idx) {
			err = atmpt.WaitingDepBitmap.Set(idx)
			impossible(err)

			if atmpt.WaitingDepBitmap.All(true) {
				atmpt.State.MustEvolve(attempt.NeedsExecution)
				muts = append(muts, &ScheduleExecution{For: f.Dep.From})
			}

			needPut = true
		}
	}

	if needPut {
		err = ds.Put(atmpt)
	}

	return
}

func init() {
	tumble.Register((*AckFwdDep)(nil))
}
