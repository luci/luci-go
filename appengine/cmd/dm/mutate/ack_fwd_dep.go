// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
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
	return datastore.Get(c).MakeKey("Attempt", f.Dep.From.DMEncoded())
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

	idx := uint32(fdep.BitIndex)

	if !atmpt.AddingDepsBitmap.IsSet(idx) {
		atmpt.AddingDepsBitmap.Set(idx)

		if atmpt.AddingDepsBitmap.All(true) {
			atmpt.MustModifyState(c, dm.Attempt_BLOCKED)
		}

		needPut = true
	}

	if f.DepIsFinished {
		if !atmpt.WaitingDepBitmap.IsSet(idx) {
			atmpt.WaitingDepBitmap.Set(idx)

			if atmpt.WaitingDepBitmap.All(true) {
				atmpt.MustModifyState(c, dm.Attempt_NEEDS_EXECUTION)
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
