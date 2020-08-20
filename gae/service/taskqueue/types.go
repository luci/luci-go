// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
