// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package model

import (
	"fmt"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/api/dm/service/v1"
	"github.com/luci/luci-go/common/bit_field"
	"github.com/luci/luci-go/common/clock"
	google_pb "github.com/luci/luci-go/common/proto/google"
)

// Attempt is the datastore model for a DM Attempt. It has no parent key, but
// it may have the following children entities:
//   * FwdDep
//   * AttemptResult
//
// Additionally, every Attempt has an associated BackDepGroup whose ID equals
// the ID of this Attempt.
type Attempt struct {
	ID dm.Attempt_ID `gae:"$id"`

	Created  time.Time
	Modified time.Time

	State dm.Attempt_State

	// TODO(iannucci): Use CurExecution as a 'deps block version'
	// then we can have an 'ANY' directive which executes the attempt as soon
	// as any of the dependencies are ready. If it adds more deps in ANY mode,
	// the bitmaps get /extended/, and new deps bit indices are added to the
	// existing max.
	// If it adds more deps in ALL mode, it just converts from ANY -> ALL mode
	// and follows the current behavior.

	// CurExecution is the maximum Execution ID for this Attempt so far. Execution
	// IDs are contiguous from [1, CurExecution]. If the State is not currently
	// Executing, then CurExecution represents the execution that JUST finished
	// (or 0 if no Executions have been made yet).
	CurExecution uint32

	// AddingDepsBitmap is valid only while Attempt is in 'AddingDeps'.
	// A field value of 0 means the backdep hasn't been added yet.
	AddingDepsBitmap bf.BitField `gae:",noindex" json:"-"`

	// WaitingDepBitmap is valid only while Attempt is in a Status of 'AddingDeps'
	// or 'Blocked'.
	// A field value of 0 means that the dep is currently waiting.
	WaitingDepBitmap bf.BitField `gae:",noindex" json:"-"`

	// Only valid while Attempt is Finished
	ResultExpiration time.Time
	ResultSize       uint32

	// A lazily-updated boolean to reflect that this Attempt is expired for
	// queries.
	Expired bool
}

// MakeAttempt is a convenience function to create a new Attempt model in
// the NeedsExecution state.
func MakeAttempt(c context.Context, aid *dm.Attempt_ID) *Attempt {
	now := clock.Now(c).UTC()
	return &Attempt{
		ID:       *aid,
		Created:  now,
		Modified: now,
	}
}

// ModifyState changes the current state of this Attempt and updates its
// Modified timestamp.
func (a *Attempt) ModifyState(c context.Context, newState dm.Attempt_State) error {
	if err := a.State.Evolve(newState); err != nil {
		return err
	}
	now := clock.Now(c).UTC()
	if now.After(a.Modified) {
		a.Modified = now
	} else {
		// Microsecond is the smallest granularity that datastore can store
		// timestamps, so use that to disambiguate: the goal here is that any
		// modification always increments the modified time, and never decrements
		// it.
		a.Modified = a.Modified.Add(time.Microsecond)
	}
	return nil
}

// MustModifyState is the same as ModifyState, except that it panics if the
// state transition is invalid.
func (a *Attempt) MustModifyState(c context.Context, newState dm.Attempt_State) {
	err := a.ModifyState(c, newState)
	if err != nil {
		panic(err)
	}
}

// ToProto returns a dm proto version of this Attempt.
func (a *Attempt) ToProto(withData bool) *dm.Attempt {
	ret := dm.Attempt{Id: &a.ID}
	if withData {
		ret.Data = a.DataProto()
	}
	return &ret
}

// DataProto returns an Attempt.Data message for this Attempt.
func (a *Attempt) DataProto() *dm.Attempt_Data {
	ret := (*dm.Attempt_Data)(nil)
	switch a.State {
	case dm.Attempt_NEEDS_EXECUTION:
		ret = dm.NewAttemptNeedsExecution(a.Modified).Data

	case dm.Attempt_EXECUTING:
		ret = dm.NewAttemptExecuting(a.CurExecution).Data

	case dm.Attempt_ADDING_DEPS:
		addset := a.AddingDepsBitmap
		waitset := a.WaitingDepBitmap
		setlen := addset.Size()

		ret = dm.NewAttemptAddingDeps(
			setlen-addset.CountSet(), setlen-waitset.CountSet()).Data

	case dm.Attempt_BLOCKED:
		waitset := a.WaitingDepBitmap
		setlen := waitset.Size()

		ret = dm.NewAttemptBlocked(setlen - waitset.CountSet()).Data

	case dm.Attempt_FINISHED:
		ret = dm.NewAttemptFinished(a.ResultExpiration, a.ResultSize, "").Data

	default:
		panic(fmt.Errorf("unknown Attempt_State: %s", a.State))
	}
	ret.Created = google_pb.NewTimestamp(a.Created)
	ret.Modified = google_pb.NewTimestamp(a.Modified)
	ret.NumExecutions = a.CurExecution
	return ret
}
