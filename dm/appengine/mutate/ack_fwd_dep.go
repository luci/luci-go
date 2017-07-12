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

package mutate

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/model"
	"github.com/luci/luci-go/tumble"
	"golang.org/x/net/context"
)

// AckFwdDep records the fact that a dependency was completed.
type AckFwdDep struct {
	Dep *model.FwdEdge
}

// Root implements tumble.Mutation.
func (f *AckFwdDep) Root(c context.Context) *ds.Key {
	return model.AttemptKeyFromID(c, f.Dep.From)
}

// RollForward implements tumble.Mutation.
func (f *AckFwdDep) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	atmpt, fdep := f.Dep.Fwd(c)
	err = ds.Get(c, atmpt, fdep)
	if err != nil {
		return
	}

	if (atmpt.State != dm.Attempt_EXECUTING && atmpt.State != dm.Attempt_WAITING) || atmpt.CurExecution != fdep.ForExecution {
		logging.Errorf(c, "EARLY EXIT: %s: %s v %s", atmpt.State, atmpt.CurExecution, fdep.ForExecution)
		return
	}

	idx := uint32(fdep.BitIndex)

	if !atmpt.DepMap.IsSet(idx) {
		atmpt.DepMap.Set(idx)

		if atmpt.DepMap.All(true) && atmpt.State == dm.Attempt_WAITING {
			if err = atmpt.ModifyState(c, dm.Attempt_SCHEDULING); err != nil {
				return
			}
			atmpt.DepMap.Reset()
			muts = append(muts, &ScheduleExecution{For: f.Dep.From})
		}

		err = ds.Put(c, atmpt)
	}

	return
}

func init() {
	tumble.Register((*AckFwdDep)(nil))
}
