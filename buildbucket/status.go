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

	v1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
)

// Status is a buildbucket build status.
// It combines Status, Result, FailureReason and CancelationReason
// used in buildbucket API.
type Status int

const (
	// Status_SCHEDULED means a build was created, but did not start or finish.
	// The initial state of a build.
	Status_SCHEDULED Status = 1 << 0
	// Status_STARTED means a build has started.
	Status_STARTED Status = 1 << 1
)

// Statuses of completed builds, aka final statuses.
// Once a build reached one of these statuses, it becomes immutable.
const (
	statusMaskCompleted Status = 1 << 2
	// Status_SUCCESS means a build has completed successfully.
	Status_SUCCESS = 1<<3 | statusMaskCompleted
	// Status_FAILURE means a build has failed due to its input,
	// for example input source code is incorrect.
	Status_FAILURE = 1<<4 | statusMaskCompleted
	// Status_INFRA_FAILURE means a build has failed not due to its input,
	// for example build infrastructure is unavailable.
	// It combines Buildbucket API v1's failure reasons "INFRA_FAILURE",
	// "BUILDBUCKET_FAILURE" and "INVALID_BUILD_DEFINITION".
	Status_INFRA_FAILURE = 1<<5 | statusMaskCompleted
	// Status_CANCELED means a build was cancelled.
	// It is equivalent to buildbucket API v1's cancelation reason
	// "CANCELED_EXPLICITLY".
	Status_CANCELED = 1<<6 | statusMaskCompleted
)

// String returns status name, e.g. "Scheduled".
func (s Status) String() string {
	switch s {
	case Status_SCHEDULED:
		return "Scheduled"
	case Status_STARTED:
		return "Started"
	case Status_SUCCESS:
		return "Success"
	case Status_FAILURE:
		return "Failure"
	case Status_INFRA_FAILURE:
		return "Error"
	case Status_CANCELED:
		return "Cancelled"
	default:
		return fmt.Sprintf("unknown status %d", s)
	}
}

// Completed returns true if s is final.
func (s Status) Completed() bool {
	return s&statusMaskCompleted == statusMaskCompleted
}

// ParseStatus parses build's Status, Result, FailureReason and
// CancelationReason as Status type.
//
// If build.Status is "", returns (0, nil). It may happen with partial
// buildbucket responses.
func ParseStatus(build *v1.ApiCommonBuildMessage) (Status, error) {
	switch build.Status {
	case "":
		return 0, nil

	case "SCHEDULED":
		return Status_SCHEDULED, nil

	case "STARTED":
		return Status_STARTED, nil

	case "COMPLETED":
		switch build.Result {
		case "SUCCESS":
			return Status_SUCCESS, nil

		case "FAILURE":
			switch build.FailureReason {
			case "", "BUILD_FAILURE":
				return Status_FAILURE, nil
			case "INFRA_FAILURE", "BUILDBUCKET_FAILURE", "INVALID_BUILD_DEFINITION":
				return Status_INFRA_FAILURE, nil
			default:
				return 0, fmt.Errorf("unexpected failure reason %q", build.FailureReason)
			}

		case "CANCELED":
			switch build.CancelationReason {
			case "", "CANCELED_EXPLICITLY":
				return Status_CANCELED, nil
			case "TIMEOUT":
				return Status_INFRA_FAILURE, nil
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
