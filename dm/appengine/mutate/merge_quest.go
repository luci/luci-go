// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutate

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/dm/appengine/model"
	"github.com/luci/luci-go/tumble"

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
