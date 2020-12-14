// Copyright 2020 The LUCI Authors.
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

package build

import (
	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// AttachStatus attaches a buildbucket status (and details) to a given error.
//
// If such an error is handled by StepState.End or State.End, that step/build
// will have its status updated to match.
//
// AttachStatus allows overriding the attached status if the error already has
// one.
//
// This panics if the status is a non-terminal status like SCHEDULED or RUNNING.
//
// This is a no-op if the error is nil.
func AttachStatus(error, bbpb.Status, *bbpb.StatusDetails) error {
	panic("implement")
}

// ExtractStatus retrieves the Buildbucket status (and details) from a given
// error.
//
// This returns:
//   * (SUCCESS, nil) on nil error
//   * Any values attached with AttachStatus.
//   * (CANCELED, nil) on context.Canceled
//   * (INFRA_FAILURE, &bbpb.StatusDetails{Timeout: {}}) on
//     context.DeadlineExceeded
//   * (FAILURE, nil) otherwise
//
// This function is used internally by StepState.End and State.End, but is
// provided publically for completeness.
func ExtractStatus(error) (bbpb.Status, *bbpb.StatusDetails) {
	panic("implement")
}
