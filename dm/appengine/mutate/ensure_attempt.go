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
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/dm/api/service/v1"
	"go.chromium.org/luci/dm/appengine/model"
	"go.chromium.org/luci/tumble"

	"golang.org/x/net/context"
)

// EnsureAttempt ensures that the given Attempt exists. If it doesn't, it's
// created in a NeedsExecution state.
type EnsureAttempt struct {
	ID *dm.Attempt_ID
}

// Root implements tumble.Mutation.
func (e *EnsureAttempt) Root(c context.Context) *ds.Key {
	return model.AttemptKeyFromID(c, e.ID)
}

// RollForward implements tumble.Mutation.
func (e *EnsureAttempt) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	a := model.MakeAttempt(c, e.ID)
	if err = ds.Get(c, a); err != ds.ErrNoSuchEntity {
		return
	}

	if err = ds.Put(c, a); err != nil {
		logging.WithError(err).Errorf(logging.SetField(c, "id", e.ID), "in put")
	}
	muts = append(muts, &ScheduleExecution{e.ID})
	return
}

func init() {
	tumble.Register((*EnsureAttempt)(nil))
}
