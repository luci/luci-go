// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
)

// FwdDep describes a 'depends-on' relation between two Attempts. It has a
// reciprocal BackDep as well, which notes the depended-on-by relationship. So:
//
//   Attempt(OTHER_QUEST|2)
//     FwdDep(QUEST|1)
//
//   Attempt(QUEST|1)
//
//   BackDepGroup(QUEST|1)
//     BackDep(OTHER_QUEST|2)
//
// Represents the OTHER_QUEST|2 depending on QUEST|1.
type FwdDep struct {
	// Attempt that this points from.
	Depender *datastore.Key `gae:"$parent"`

	// A FwdDep's ID is the Attempt ID that it points to.
	Dependee types.AttemptID `gae:"$id"`

	// This will be used to set a bit in the Attempt (WaitingDepBitmap) when the
	// Dep completes.
	BitIndex types.UInt32

	// ForExecution indicates which Execution added this dependency, mostly for
	// historical analysis/display.
	ForExecution types.UInt32
}

// Edge produces a edge object which points 'forwards' from the depending
// attempt to the depended-on attempt.
func (f *FwdDep) Edge() *FwdEdge {
	return &FwdEdge{
		From: types.NewAttemptID(f.Depender.StringID()),
		To:   &f.Dependee,
	}
}
