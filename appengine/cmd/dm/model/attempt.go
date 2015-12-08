// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"time"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/cmd/dm/display"
	"github.com/luci/luci-go/appengine/cmd/dm/enums/attempt"
	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/bit_field"
)

// Attempt is the datastore model for a DM Attempt. It has no parent key, but
// it may have the following children entities:
//   * FwdDep
//   * AttemptResult
//
// Additionally, every Attempt has an associated BackDepGroup whose ID equals
// the ID of this Attempt.
type Attempt struct {
	types.AttemptID `gae:"$id"`

	State attempt.State

	// TODO(iannucci): Use CurExecution as a 'deps block version'
	// then we can have an 'ANY' directive which executes the attempt as soon
	// as any of the dependencies are ready. If it adds more deps in ANY mode,
	// the bitmaps get /extended/, and new deps bit indices are added to the
	// existing max.
	// If it adds more deps in ALL mode, it just converts from ANY -> ALL mode
	// and follows the current behavior.

	// CurExecution is the maximum Execution ID for this Attempt so far. Execution
	// IDs are contiguous from [0, CurExecution]
	CurExecution types.UInt32

	// AddingDepsBitmap is valid only while Attempt is in 'AddingDeps'.
	// A field value of 0 means the backdep hasn't been added yet.
	AddingDepsBitmap bf.BitField `gae:",noindex" json:"-"`

	// WaitingDepBitmap is valid only while Attempt is in a Status of 'AddingDeps'
	// or 'Blocked'.
	// A field value of 0 means that the dep is currently waiting.
	WaitingDepBitmap bf.BitField `gae:",noindex" json:"-"`

	// Only valid while Attempt is Finished
	ResultExpiration time.Time
}

var _ datastore.PropertyLoadSaver = (*Attempt)(nil)

// Load implements datastore.PropertyLoadSaver
func (a *Attempt) Load(pm datastore.PropertyMap) error {
	// nil out these bitmaps so that they don't get append'd to during the
	// load procedure.
	a.AddingDepsBitmap.Data = nil
	a.WaitingDepBitmap.Data = nil
	return datastore.GetPLS(a).Load(pm)
}

// Save implements datastore.PropertyLoadSaver
func (a *Attempt) Save(withMeta bool) (datastore.PropertyMap, error) {
	return datastore.GetPLS(a).Save(withMeta)
}

// Problem implements datastore.PropertyLoadSaver
func (a *Attempt) Problem() error {
	return datastore.GetPLS(a).Problem()
}

// NewAttempt creates a new Attempt for the given quest with the specified
// Attempt number. It is created with the UNKNOWN state.
func NewAttempt(quest string, num uint32) *Attempt {
	return &Attempt{AttemptID: types.AttemptID{QuestID: quest, AttemptNum: num}}
}

// ToDisplay returns a display.Attempt for this Attempt.
func (a *Attempt) ToDisplay() *display.Attempt {
	return &display.Attempt{
		ID: a.AttemptID,

		NumExecutions: uint32(a.CurExecution),
		State:         a.State,
		Expiration:    a.ResultExpiration,
		NumWaitingDeps: uint32(
			a.WaitingDepBitmap.Size() - a.WaitingDepBitmap.CountSet()),
	}
}
