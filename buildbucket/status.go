// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"fmt"

	"go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
)

// Status is a buildbucket build status.
// It combines Status, Result, FailureReason and CancelationReason
// used in buildbucket API.
type Status int

const (
	// StatusScheduled means a build was created, but did not start or finish.
	// The initial state of a build.
	StatusScheduled Status = iota
	// StatusStarted means a build has started.
	StatusStarted
	completedStart
	// StatusSuccess means a build has completed successfully.
	// A final state.
	StatusSuccess
	failureStart
	// StatusFailure means a build has failed due to its input,
	// for example input source code is incorrect.
	// A final state.
	StatusFailure
	// StatusInfraFailure means a build has failed not due to its input,
	// for example build infrastructure is unavailable.
	// It combines Buildbucket API v1's failure reasons "INFRA_FAILURE",
	// "BUILDBUCKET_FAILURE" and "INVALID_BUILD_DEFINITION".
	// A final state.
	StatusInfraFailure
	failureEnd
	// StatusCancelled means a build was cancelled.
	// It is equivalent to buildbucket API v1's cancelation reason
	// "CANCELED_EXPLICITLY".
	// A final state.
	StatusCancelled
	// StatusTimeout means a build did not reach other final status in time.
	// It is equivalent to buildbucket API v1's cancelation reason "TIMEOUT".
	// A final state.
	StatusTimeout
)

// String returns status name, e.g. "Scheduled".
func (s Status) String() string {
	switch s {
	case StatusScheduled:
		return "Scheduled"
	case StatusStarted:
		return "Started"
	case StatusSuccess:
		return "Success"
	case StatusFailure:
		return "Failure"
	case StatusInfraFailure:
		return "InfraFailure"
	case StatusCancelled:
		return "Cancelled"
	case StatusTimeout:
		return "Timeout"
	default:
		return fmt.Sprintf("unknown status %d", s)
	}
}

// Completed returns true if s is final.
func (s Status) Completed() bool {
	return s > completedStart
}

// Failure returns true if s means the build has failed
// due to input or infrastructure.
func (s Status) Failure() bool {
	return s > failureStart && s < failureEnd
}

// ParseStatus parses build's Status, Result, FailureReason and
// CancelationReason as Status type.
func ParseStatus(build *buildbucket.ApiCommonBuildMessage) (Status, error) {
	switch build.Status {
	case "SCHEDULED":
		return StatusScheduled, nil

	case "STARTED":
		return StatusStarted, nil

	case "COMPLETED":
		switch build.Result {
		case "SUCCESS":
			return StatusSuccess, nil

		case "FAILURE":
			switch build.FailureReason {
			case "", "BUILD_FAILURE":
				return StatusFailure, nil
			case "INFRA_FAILURE", "BUILDBUCKET_FAILURE", "INVALID_BUILD_DEFINITION":
				return StatusInfraFailure, nil
			default:
				return 0, fmt.Errorf("unexpected failure reason %q", build.FailureReason)
			}

		case "CANCELED":
			switch build.CancelationReason {
			case "", "CANCELED_EXPLICITLY":
				return StatusCancelled, nil
			case "TIMEOUT":
				return StatusTimeout, nil
			default:
				return 0, fmt.Errorf("unexpected cancellation reason %q", build.CancelationReason)
			}

		default:
			return 0, fmt.Errorf("unexpected result %q", build.Result)
		}

	default:
		return 0, fmt.Errorf("unexpected status %q", build.Status)
	}
}
