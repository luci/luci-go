// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"time"

	"google.golang.org/grpc/codes"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/grpcutil"
	"golang.org/x/net/context"
)

// FinishAttempt does a couple things:
//   Invalidates the current Execution
//   Moves the state to Finished
//   Creates a new AttemptResult
//   Starts RecordCompletion state machine.
type FinishAttempt struct {
	Auth             *dm.Execution_Auth
	Result           string
	ResultExpiration time.Time
}

// Root implements tumble.Mutation
func (f *FinishAttempt) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).KeyForObj(&model.Attempt{ID: *f.Auth.Id.AttemptID()})
}

// RollForward implements tumble.Mutation
//
// This mutation is called directly from FinishAttempt, so we use
// grpcutil.MaybeLogErr
func (f *FinishAttempt) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	atmpt, _, err := model.InvalidateExecution(c, f.Auth)
	if err != nil {
		return
	}

	ds := datastore.Get(c)

	// Executing -> Finished is valid, and we know we're already Executing because
	// the InvalidateExecution call above asserts that or errors out.
	atmpt.MustModifyState(c, dm.Attempt_FINISHED)

	atmpt.ResultSize = uint32(len(f.Result))
	atmpt.ResultExpiration = f.ResultExpiration
	rslt := &model.AttemptResult{
		Attempt:    ds.KeyForObj(atmpt),
		Data:       f.Result,
		Expiration: atmpt.ResultExpiration,
		Size:       atmpt.ResultSize,
	}

	err = grpcutil.MaybeLogErr(c, ds.PutMulti([]interface{}{atmpt, rslt}),
		codes.Internal, "while trying to PutMulti")

	// TODO(iannucci): also include mutations to generate index entries for
	// the attempt results.
	muts = append(muts, &RecordCompletion{For: f.Auth.Id.AttemptID()})

	return
}

func init() {
	tumble.Register((*FinishAttempt)(nil))
}
