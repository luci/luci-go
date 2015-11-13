// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate stringer -type=AttemptState

package types

// AttemptState is the enumeration of possible states for an Attempt.
type AttemptState int8

// These are the only valid enumerated values for AttemptState.
const (
	UNKNOWN AttemptState = iota
	NeedsExecution
	Executing
	AddingDeps
	Blocked
	Finished
)
