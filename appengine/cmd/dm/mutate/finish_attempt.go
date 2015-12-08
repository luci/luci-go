// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutate

import (
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/enums/attempt"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/appengine/tumble"
	"golang.org/x/net/context"
)

// FinishAttempt does a couple things:
//   Invalidates the current Execution
//   Moves the state to Finished
//   Creates a new AttemptResult
//   Starts RecordCompletion state machine.
type FinishAttempt struct {
	ID               types.AttemptID
	ExecutionKey     []byte
	Result           []byte
	ResultExpiration time.Time
}

// Root implements tumble.Mutation
func (f *FinishAttempt) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).KeyForObj(&model.Attempt{AttemptID: f.ID})
}

// RollForward implements tumble.Mutation
func (f *FinishAttempt) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	atmpt, _, err := model.InvalidateExecution(c, &f.ID, f.ExecutionKey)
	if err != nil {
		return
	}

	ds := datastore.Get(c)

	// Executing -> Finished is valid, and we know we're already Executing because
	// the InvalidateExecution call above asserts that or errors out.
	atmpt.State.MustEvolve(attempt.Finished)

	rslt := &model.AttemptResult{
		Attempt: ds.KeyForObj(atmpt),
		Data:    f.Result,
	}
	atmpt.ResultExpiration = f.ResultExpiration

	err = ds.PutMulti([]interface{}{atmpt, rslt})

	// TODO(iannucci): also include mutations to generate index entries for
	// the attempt results.
	muts = append(muts, &RecordCompletion{For: f.ID})

	return
}

func init() {
	tumble.Register((*FinishAttempt)(nil))
}
