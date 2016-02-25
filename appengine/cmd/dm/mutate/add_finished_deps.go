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

// AddFinishedDeps adds a bunch of dependencies which are known in advance to
// already be in the Finished state.
type AddFinishedDeps struct {
	Auth  *dm.Execution_Auth
	ToAdd *dm.AttemptFanout
}

// Root implements tumble.Mutation
func (f *AddFinishedDeps) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).KeyForObj(&model.Attempt{ID: *f.Auth.Id.AttemptID()})
}

// RollForward implements tumble.Mutation
func (f *AddFinishedDeps) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	atmpt, _, err := model.AuthenticateExecution(c, f.Auth)
	if err != nil {
		return
	}

	fwdDeps, err := filterExisting(c, model.FwdDepsFromFanout(c, f.Auth.Id.AttemptID(), f.ToAdd))
	if err != nil || len(fwdDeps) == 0 {
		return
	}

	muts = make([]tumble.Mutation, len(fwdDeps))
	for i, d := range fwdDeps {
		d.ForExecution = atmpt.CurExecution
		muts[i] = &AddBackDep{Dep: d.Edge()}
	}

	return muts, datastore.Get(c).PutMulti(fwdDeps)
}

func init() {
	tumble.Register((*AddFinishedDeps)(nil))
}
