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

package runtest

import (
	"go.chromium.org/luci/cv/internal/run"
)

// AreRunning is true if all runs are non-nil and Running.
func AreRunning(runs ...*run.Run) bool {
	for _, r := range runs {
		if r == nil || r.Status != run.Status_RUNNING {
			return false
		}
	}
	return true
}

// AreEnded is true if all runs are not nil and have ended.
func AreEnded(runs ...*run.Run) bool {
	for _, r := range runs {
		if r == nil || !run.IsEnded(r.Status) {
			return false
		}
	}
	return true
}

// FilterNot returns non-nil Runs whose status differs from the given one.
//
// If given ENDED_MASK status, returns all Runs which haven't ended yet.
func FilterNot(status run.Status, runs ...*run.Run) []*run.Run {
	var left []*run.Run
	for _, r := range runs {
		switch {
		case r == nil:
		case r.Status == status:
		case status == run.Status_ENDED_MASK && run.IsEnded(r.Status):
		default:
			left = append(left, r)
		}
	}
	return left
}
