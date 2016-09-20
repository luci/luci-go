// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutate

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/dm/api/service/v1"
	"github.com/luci/luci-go/dm/appengine/model"
	"github.com/luci/luci-go/tumble"

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
