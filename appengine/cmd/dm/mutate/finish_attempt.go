// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	return model.AttemptKeyFromID(c, f.Auth.Id.AttemptID())
}

// RollForward implements tumble.Mutation
//
// This mutation is called directly from FinishAttempt, so we use
// grpcutil.MaybeLogErr
func (f *FinishAttempt) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	atmpt, ex, err := model.InvalidateExecution(c, f.Auth)
	if err != nil {
		return
	}

	if err = ResetExecutionTimeout(c, ex); err != nil {
		return
	}

	ds := datastore.Get(c)

	atmpt.ResultSize = uint32(len(f.Result))
	atmpt.ResultExpiration = f.ResultExpiration
	rslt := &model.AttemptResult{
		Attempt:    ds.KeyForObj(atmpt),
		Data:       f.Result,
		Expiration: atmpt.ResultExpiration,
		Size:       atmpt.ResultSize,
	}

	err = grpcutil.MaybeLogErr(c, ds.Put(atmpt, rslt),
		codes.Internal, "while trying to Put")

	return
}

func init() {
	tumble.Register((*FinishAttempt)(nil))
}
