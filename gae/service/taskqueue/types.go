// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file contains types which are mirrors/duplicates of the upstream SDK
// types. This exists so that users can depend solely on this wrapper library
// without necessarially needing an SDK implementation present.
//
// This was done (instead of type-aliasing from the github version of the SDK)
// because some of the types need to be tweaked (like Task.RetryOptions) to
// interact well with the wrapper, and the inconsistency of having some types
// defined by the gae package and others defined by the SDK was pretty awkward.

package taskqueue

import (
	"time"
)

// Statistics represents statistics about a single task queue.
type Statistics struct {
	Tasks     int       // may be an approximation
	OldestETA time.Time // zero if there are no pending tasks

	Executed1Minute int     // tasks executed in the last minute
	InFlight        int     // tasks executing now
	EnforcedRate    float64 // requests per second
}
