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
	"github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/model"
	"github.com/luci/luci-go/tumble"
	"golang.org/x/net/context"
)

// AddFinishedDeps adds a bunch of dependencies which are known in advance to
// already be in the Finished state.
type AddFinishedDeps struct {
	Auth *dm.Execution_Auth

	// MergeQuests lists quests which need their BuiltBy lists merged. The Quests
	// here must be a subset of the quests mentioned in FinishedAttempts.
	MergeQuests []*model.Quest

	// FinishedAttempts are a list of attempts that we already know are in the
	// Finished state.
	FinishedAttempts *dm.AttemptList
}

// Root implements tumble.Mutation
func (f *AddFinishedDeps) Root(c context.Context) *ds.Key {
	return model.AttemptKeyFromID(c, f.Auth.Id.AttemptID())
}

// RollForward implements tumble.Mutation
func (f *AddFinishedDeps) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	atmpt, _, err := model.AuthenticateExecution(c, f.Auth)
	if err != nil {
		return
	}

	fwdDeps, err := filterExisting(c, model.FwdDepsFromList(c, f.Auth.Id.AttemptID(), f.FinishedAttempts))
	if err != nil || len(fwdDeps) == 0 {
		return
	}

	muts = make([]tumble.Mutation, 0, len(fwdDeps)+len(f.MergeQuests))
	for _, d := range fwdDeps {
		d.ForExecution = atmpt.CurExecution
		muts = append(muts, &AddBackDep{Dep: d.Edge()})
	}
	for _, q := range f.MergeQuests {
		muts = append(muts, &MergeQuest{Quest: q})
	}

	return muts, ds.Put(c, fwdDeps)
}

func init() {
	tumble.Register((*AddFinishedDeps)(nil))
}
