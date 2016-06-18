// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutate

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"golang.org/x/net/context"
)

// AckFwdDep records the fact that a dependency was completed.
type AckFwdDep struct {
	Dep *model.FwdEdge
}

// Root implements tumble.Mutation.
func (f *AckFwdDep) Root(c context.Context) *datastore.Key {
	return model.AttemptKeyFromID(c, f.Dep.From)
}

// RollForward implements tumble.Mutation.
func (f *AckFwdDep) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	ds := datastore.Get(c)

	atmpt, fdep := f.Dep.Fwd(c)
	err = ds.Get(atmpt, fdep)
	if err != nil {
		return
	}

	if atmpt.State != dm.Attempt_WAITING || atmpt.CurExecution != fdep.ForExecution {
		return
	}

	idx := uint32(fdep.BitIndex)

	if !atmpt.DepMap.IsSet(idx) {
		atmpt.DepMap.Set(idx)

		if atmpt.DepMap.All(true) {
			if err = atmpt.ModifyState(c, dm.Attempt_SCHEDULING); err != nil {
				return
			}
			atmpt.DepMap.Reset()
			muts = append(muts, &ScheduleExecution{For: f.Dep.From})
		}

		err = ds.Put(atmpt)
	}

	return
}

func init() {
	tumble.Register((*AckFwdDep)(nil))
}
