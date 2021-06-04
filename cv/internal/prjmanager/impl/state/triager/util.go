// Copyright 2021 The LUCI Authors.
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

package triager

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// earliest returns the earliest of the non-zero time instances.
//
// Returns zero time.Time if no non-zero instances were given.
func earliest(ts ...time.Time) time.Time {
	var res time.Time
	var remaining []time.Time
	// Find first non-zero.
	for i, t := range ts {
		if !t.IsZero() {
			res = t
			remaining = ts[i+1:]
			break
		}
	}
	for _, t := range remaining {
		// res is guaranteed non-zero if this loop iterates.
		if !t.IsZero() && t.Before(res) {
			res = t
		}
	}
	return res
}

func isSameTime(t time.Time, pb *timestamppb.Timestamp) bool {
	if pb == nil {
		return t.IsZero()
	}
	return pb.AsTime().Equal(t)
}
