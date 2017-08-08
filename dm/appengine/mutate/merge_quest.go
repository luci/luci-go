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
	"go.chromium.org/luci/dm/appengine/model"
	"go.chromium.org/luci/tumble"

	"golang.org/x/net/context"
)

// MergeQuest ensures that the given Quest exists and contains the merged
// set of BuiltBy entries.
type MergeQuest struct {
	Quest   *model.Quest
	AndThen []tumble.Mutation
}

// Root implements tumble.Mutation.
func (m *MergeQuest) Root(c context.Context) *ds.Key {
	return model.QuestKeyFromID(c, m.Quest.ID)
}

// RollForward implements tumble.Mutation.
func (m *MergeQuest) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	curQuest := model.QuestFromID(m.Quest.ID)

	c = logging.SetField(c, "qid", m.Quest.ID)

	reason := "getting quest"
	switch err = ds.Get(c, curQuest); err {
	case nil:
		prevLen := len(curQuest.BuiltBy)
		curQuest.BuiltBy.Add(m.Quest.BuiltBy...)
		if len(curQuest.BuiltBy) > prevLen {
			reason = "putting merged quest"
			err = ds.Put(c, curQuest)
		}
	case ds.ErrNoSuchEntity:
		reason = "putting quest"
		err = ds.Put(c, m.Quest)
	}

	if err != nil {
		logging.WithError(err).Errorf(c, "%s", reason)
	}

	muts = m.AndThen

	return
}

func init() {
	tumble.Register((*MergeQuest)(nil))
}
