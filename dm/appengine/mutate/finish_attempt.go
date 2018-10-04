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
	"context"

	"google.golang.org/grpc/codes"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	dm "go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/tumble"
)

// FinishAttempt does a couple things:
//   Invalidates the current Execution
//   Moves the state to Finished
//   Creates a new AttemptResult
//   Starts RecordCompletion state machine.
type FinishAttempt struct {
	dm.FinishAttemptReq
}

// Root implements tumble.Mutation
func (f *FinishAttempt) Root(c context.Context) *ds.Key {
	return model.AttemptKeyFromID(c, f.Auth.Id.AttemptID())
}

// RollForward implements tumble.Mutation
//
// This mutation is called directly from FinishAttempt.
func (f *FinishAttempt) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	atmpt, ex, err := model.InvalidateExecution(c, f.Auth)
	if err != nil {
		logging.WithError(err).Errorf(c, "could not invalidate execution")
		return
	}

	if err = ResetExecutionTimeout(c, ex); err != nil {
		logging.WithError(err).Errorf(c, "could not reset timeout")
		return
	}

	ar := &model.AttemptResult{
		Attempt: model.AttemptKeyFromID(c, &atmpt.ID),
		Data:    *f.Data,
	}

	rslt := *f.Data
	atmpt.Result.Data = &rslt
	atmpt.Result.Data.Object = ""

	err = errors.Annotate(ds.Put(c, atmpt, ar), "during Put").Tag(grpcutil.Tag.With(codes.Internal)).Err()
	return
}

func init() {
	tumble.Register((*FinishAttempt)(nil))
}
