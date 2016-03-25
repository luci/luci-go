// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dm

import (
	"time"

	google_pb "github.com/luci/luci-go/common/proto/google"
)

// NewAttemptNeedsExecution creates an Attempt in the NeedsExecution state.
func NewAttemptNeedsExecution(pending time.Time) *Attempt {
	return &Attempt{
		Data: &Attempt_Data{
			AttemptType: &Attempt_Data_NeedsExecution_{
				NeedsExecution: &Attempt_Data_NeedsExecution{
					google_pb.NewTimestamp(pending)}}}}
}

// NewAttemptExecuting creates an Attempt in the Executing state.
func NewAttemptExecuting(curExID uint32) *Attempt {
	return &Attempt{
		Data: &Attempt_Data{
			AttemptType: &Attempt_Data_Executing_{
				Executing: &Attempt_Data_Executing{
					curExID}}}}
}

// NewAttemptAddingDeps creates an Attempt in the AddingDeps state.
func NewAttemptAddingDeps(numAdding, numWaiting uint32) *Attempt {
	return &Attempt{
		Data: &Attempt_Data{
			AttemptType: &Attempt_Data_AddingDeps_{
				AddingDeps: &Attempt_Data_AddingDeps{
					numAdding, numWaiting}}}}
}

// NewAttemptBlocked creates an Attempt in the Blocked state.
func NewAttemptBlocked(numWaiting uint32) *Attempt {
	return &Attempt{
		Data: &Attempt_Data{
			AttemptType: &Attempt_Data_Blocked_{
				Blocked: &Attempt_Data_Blocked{
					numWaiting}}}}
}

// NewAttemptFinished creates an Attempt in the Finished state.
func NewAttemptFinished(expiration time.Time, jsonResultSize uint32, jsonResult string) *Attempt {
	return &Attempt{
		Data: &Attempt_Data{
			AttemptType: &Attempt_Data_Finished_{
				Finished: &Attempt_Data_Finished{
					google_pb.NewTimestamp(expiration), jsonResultSize, jsonResult}}}}
}

// State computes the Attempt_State for the current Attempt_Data
func (d *Attempt_Data) State() Attempt_State {
	switch d.AttemptType.(type) {
	case *Attempt_Data_Executing_:
		return Attempt_Executing
	case *Attempt_Data_AddingDeps_:
		return Attempt_AddingDeps
	case *Attempt_Data_Blocked_:
		return Attempt_Blocked
	case *Attempt_Data_Finished_:
		return Attempt_Finished
	}
	// NeedsExecution is the default
	return Attempt_NeedsExecution
}

// NormalizePartial will nil out the Partial field for this Attempt if all
// Partial fields are false.
func (d *Attempt) NormalizePartial() {
	p := d.GetPartial()
	if p == nil {
		return
	}
	if !(p.Data || p.Executions || p.FwdDeps || p.BackDeps ||
		p.Result != Attempt_Partial_Loaded) {
		d.Partial = nil
	}
}
