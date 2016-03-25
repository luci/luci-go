// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/bit_field"
	"github.com/luci/luci-go/common/grpcutil"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// AddDeps transactionally stops the current execution and adds one or more
// dependencies. It assumes that, prior to execution, all Quests named by ToAdd
// have already been recorded globally.
type AddDeps struct {
	Auth  *dm.Execution_Auth
	ToAdd *dm.AttemptList
}

// Root implements tumble.Mutation
func (a *AddDeps) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).KeyForObj(&model.Attempt{ID: *a.Auth.Id.AttemptID()})
}

// RollForward implements tumble.Mutation
func (a *AddDeps) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	// Invalidate the execution key so that they can't make more API calls.
	atmpt, _, err := model.InvalidateExecution(c, a.Auth)
	if err != nil {
		return
	}

	fwdDeps, err := filterExisting(c, model.FwdDepsFromList(c, a.Auth.Id.AttemptID(), a.ToAdd))
	if err != nil || len(fwdDeps) == 0 {
		return
	}

	ds := datastore.Get(c)

	atmpt.AddingDepsBitmap = bf.Make(uint32(len(fwdDeps)))
	atmpt.WaitingDepBitmap = bf.Make(uint32(len(fwdDeps)))
	atmpt.MustModifyState(c, dm.Attempt_ADDING_DEPS)

	for i, fdp := range fwdDeps {
		fdp.BitIndex = uint32(i)
		fdp.ForExecution = atmpt.CurExecution
	}
	if err = ds.PutMulti(fwdDeps); err != nil {
		logging.WithError(err).Errorf(c, "error putting new fwdDeps")
		err = grpcutil.Internal
		return
	}

	if err = ds.Put(atmpt); err != nil {
		logging.WithError(err).Errorf(c, "error putting attempt")
		err = grpcutil.Internal
		return
	}

	muts = make([]tumble.Mutation, 0, 2*len(fwdDeps))
	for i, d := range fwdDeps {
		d.BitIndex = uint32(i)
		muts = append(muts, &EnsureAttempt{ID: &d.Dependee})
		muts = append(muts, &AddBackDep{
			Dep:      d.Edge(),
			NeedsAck: true,
		})
	}

	return
}

func init() {
	tumble.Register((*AddDeps)(nil))
}
