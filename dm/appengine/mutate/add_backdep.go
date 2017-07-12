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
	"github.com/luci/luci-go/dm/appengine/model"
	"github.com/luci/luci-go/tumble"
	"golang.org/x/net/context"
)

// AddBackDep adds a BackDep (and possibly a BackDepGroup). If NeedsAck
// is true, this mutation will chain to an AckFwdDep. It should only be false
// if this AddBackDep is spawned from an AddFinishedDeps, where the originating
// Attempt already knows that this dependency is Finished.
type AddBackDep struct {
	Dep      *model.FwdEdge
	NeedsAck bool // make AckFwdDep iff true
}

// Root implements tumble.Mutation.
func (a *AddBackDep) Root(c context.Context) *ds.Key {
	bdg, _ := a.Dep.Back(c)
	return ds.KeyForObj(c, bdg)
}

// RollForward implements tumble.Mutation.
func (a *AddBackDep) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	bdg, bd := a.Dep.Back(c)
	err = ds.Get(c, bdg)
	if err == ds.ErrNoSuchEntity {
		err = ds.Put(c, bdg)
	}
	if err != nil {
		return
	}

	bd.Propagated = bdg.AttemptFinished

	err = ds.Put(c, bd)

	if a.NeedsAck && bdg.AttemptFinished {
		muts = append(muts, &AckFwdDep{a.Dep})
	}
	return
}

func init() {
	tumble.Register((*AddBackDep)(nil))
}
