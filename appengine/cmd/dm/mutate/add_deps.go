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
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
)

// AddDeps transactionally stops the current execution and adds one or more
// dependencies. It assumes that, prior to execution, all Quests named by Deps
// have already been recorded globally.
type AddDeps struct {
	Auth   *dm.Execution_Auth
	Quests []*model.Quest

	// Atmpts is attempts we think are missing from the global graph.
	Atmpts *dm.AttemptList

	// Deps are fwddeps we think are missing from the auth'd attempt.
	Deps *dm.AttemptList
}

// Root implements tumble.Mutation
func (a *AddDeps) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).KeyForObj(&model.Attempt{ID: *a.Auth.Id.AttemptID()})
}

// RollForward implements tumble.Mutation
//
// This mutation is called directly, so we use return grpc errors.
func (a *AddDeps) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	// Invalidate the execution key so that they can't make more API calls.
	atmpt, _, err := model.InvalidateExecution(c, a.Auth)
	if err != nil {
		return
	}

	fwdDeps, err := filterExisting(c, model.FwdDepsFromList(c, a.Auth.Id.AttemptID(), a.Deps))
	err = grpcutil.MaybeLogErr(c, err, codes.Internal, "while filtering deps")
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
		err = grpcutil.MaybeLogErr(c, err, codes.Internal, "error putting new fwdDeps")
		return
	}
	if err = ds.Put(atmpt); err != nil {
		err = grpcutil.MaybeLogErr(c, err, codes.Internal, "error putting attempt")
		return
	}

	muts = make([]tumble.Mutation, 0, len(fwdDeps)+len(a.Atmpts.GetTo()))
	for _, d := range fwdDeps {
		if nums, ok := a.Atmpts.GetTo()[d.Dependee.Quest]; ok {
			for _, n := range nums.Nums {
				if n == d.Dependee.Id {
					muts = append(muts, &EnsureAttempt{ID: &d.Dependee})
					break
				}
			}
		}
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
