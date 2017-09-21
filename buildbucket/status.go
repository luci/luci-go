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
	StatusScheduled Status = iota
	// StatusStarted means a build has started.
	StatusStarted
	completedStart
	// StatusSuccess means a build has completed successfully.
	StatusSuccess
	failureStart
	// StatusFailure means a build has failed due to its input,
	// for example input source code is incorrect.
	StatusFailure
	// StatusInfraFailure means a build has failed not due to its input,
	// for example build infrastructure is unavailable.
	// It combines failure reasons "INFRA_FAILURE", "BUILDBUCKET_FAILURE"
	// and "INVALID_BUILD_DEFINITION".
	StatusInfraFailure
	failureEnd
	// StatusCancelled means a build was cancelled.
	StatusCancelled
	// StatusTimeout means a build did not reach terminal status in time.
	StatusTimeout
)

// Completed returns true if s is terminal.
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
			case "BUILD_FAILURE":
				return StatusFailure, nil
			case "INFRA_FAILURE", "BUILDBUCKET_FAILURE", "INVALID_BUILD_DEFINITION":
				return StatusInfraFailure, nil
			default:
				return 0, fmt.Errorf("unexpected failure reason %q", build.FailureReason)
			}

		case "CANCELED":
			switch build.CancelationReason {
			case "CANCELED_EXPLICITLY":
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
