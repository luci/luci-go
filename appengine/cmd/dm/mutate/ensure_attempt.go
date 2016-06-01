// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package mutate

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/model"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// EnsureAttempt ensures that the given Attempt exists. If it doesn't, it's
// created in a NeedsExecution state.
type EnsureAttempt struct {
	ID *dm.Attempt_ID
}

// Root implements tumble.Mutation.
func (e *EnsureAttempt) Root(c context.Context) *datastore.Key {
	return datastore.Get(c).KeyForObj(&model.Attempt{ID: *e.ID})
}

// RollForward implements tumble.Mutation.
func (e *EnsureAttempt) RollForward(c context.Context) (muts []tumble.Mutation, err error) {
	ds := datastore.Get(c)

	a := model.MakeAttempt(c, e.ID)
	err = ds.Get(a)
	if err != datastore.ErrNoSuchEntity {
		return
	}

	if err = ds.Put(a); err != nil {
		logging.WithError(err).Errorf(logging.SetField(c, "id", e.ID), "in put")
	}
	return
}

func init() {
	tumble.Register((*EnsureAttempt)(nil))
}
