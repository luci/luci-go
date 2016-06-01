// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutate

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// MergeQuest ensures that the given Quest exists and contains the merged
// set of BuiltBy entries.
type MergeQuest struct {
	Quest *model.Quest
}

// Root implements tumble.Mutation.
func (m *MergeQuest) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).KeyForObj(m.Quest)
}

// RollForward implements tumble.Mutation.
func (m *MergeQuest) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	ds := datastore.Get(c)

	curQuest := &model.Quest{ID: m.Quest.ID}

	c = logging.SetField(c, "qid", m.Quest.ID)

	reason := "getting quest"
	switch err = ds.Get(curQuest); err {
	case nil:
		prevLen := len(curQuest.BuiltBy)
		curQuest.BuiltBy.Add(m.Quest.BuiltBy...)
		if len(curQuest.BuiltBy) > prevLen {
			reason = "putting merged quest"
			err = ds.Put(curQuest)
		}
	case datastore.ErrNoSuchEntity:
		reason = "putting quest"
		err = ds.Put(m.Quest)
	}

	if err != nil {
		logging.WithError(err).Errorf(c, "%s", reason)
	}

	return
}

func init() {
	tumble.Register((*MergeQuest)(nil))
}
