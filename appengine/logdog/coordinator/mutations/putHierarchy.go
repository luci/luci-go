// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mutations

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/logdog/types"
	"golang.org/x/net/context"
)

// PutHierarchyMutation is a tumble Mutation that registers a single hierarchy
// component.
type PutHierarchyMutation struct {
	// Path is the path of the log stream to add to the hierarchy.
	Path types.StreamPath
}

// RollForward implements tumble.Mutation.
func (m *PutHierarchyMutation) RollForward(c context.Context) ([]tumble.Mutation, error) {
	return nil, hierarchy.Put(ds.Get(c), m.Path)

}

// Root implements tumble.Mutation.
func (m *PutHierarchyMutation) Root(c context.Context) *ds.Key {
	return hierarchy.Root(ds.Get(c))
}

func init() {
	tumble.Register((*PutHierarchyMutation)(nil))
}
