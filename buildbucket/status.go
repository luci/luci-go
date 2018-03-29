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

	"go.chromium.org/luci/buildbucket/proto"

	v1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
)

// ParseStatus parses build's Status, Result, FailureReason and
// CancelationReason as Status type.
//
// If build.Status is "", returns (0, nil). It may happen with partial
// buildbucket responses.
func ParseStatus(build *v1.ApiCommonBuildMessage) (buildbucketpb.Status, error) {
	switch build.Status {
	case "":
		return buildbucketpb.Status_STATUS_UNSPECIFIED, nil

	case "SCHEDULED":
		return buildbucketpb.Status_SCHEDULED, nil

	case "STARTED":
		return buildbucketpb.Status_STARTED, nil

	case "COMPLETED":
		switch build.Result {
		case "SUCCESS":
			return buildbucketpb.Status_SUCCESS, nil

		case "FAILURE":
			switch build.FailureReason {
			case "", "BUILD_FAILURE":
				return buildbucketpb.Status_FAILURE, nil
			case "INFRA_FAILURE", "BUILDBUCKET_FAILURE", "INVALID_BUILD_DEFINITION":
				return buildbucketpb.Status_INFRA_FAILURE, nil
			default:
				return 0, fmt.Errorf("unexpected failure reason %q", build.FailureReason)
			}

		case "CANCELED":
			switch build.CancelationReason {
			case "", "CANCELED_EXPLICITLY":
				return buildbucketpb.Status_CANCELED, nil
			case "TIMEOUT":
				return buildbucketpb.Status_INFRA_FAILURE, nil
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
