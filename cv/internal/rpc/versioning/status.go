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

package versioning

import (
	"errors"
	"fmt"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	apiv1pb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// TryjobStatusV0 converts internal Tryjob statuses to an APIv0 equivalent.
func TryjobStatusV0(tjs tryjob.Status, tjrs tryjob.Result_Status) apiv0pb.TryjobStatus {
	switch tjs {
	case tryjob.Status_STATUS_UNSPECIFIED:
		panic(errors.New("tryjob status not specified"))
	case tryjob.Status_PENDING:
		return apiv0pb.TryjobStatus_PENDING
	case tryjob.Status_TRIGGERED:
		return apiv0pb.TryjobStatus_RUNNING
	case tryjob.Status_ENDED:
		switch tjrs {
		case tryjob.Result_RESULT_STATUS_UNSPECIFIED:
			panic(errors.New("tryjob result status not specified"))
		case tryjob.Result_SUCCEEDED:
			return apiv0pb.TryjobStatus_SUCCEEDED
		case tryjob.Result_FAILED_TRANSIENTLY, tryjob.Result_FAILED_PERMANENTLY, tryjob.Result_TIMEOUT:
			return apiv0pb.TryjobStatus_FAILED
		default:
			panic(fmt.Sprintf("unknown tryjob result status: %s", tjrs))
		}
	case tryjob.Status_CANCELLED:
		return apiv0pb.TryjobStatus_CANCELLED
	case tryjob.Status_UNTRIGGERED:
		return apiv0pb.TryjobStatus_UNTRIGGERED
	default:
		panic(fmt.Sprintf("unknown tryjob status: %s", tjs))
	}
}

// LegacyTryjobStatusV0 converts internal Tryjob status to an APIv0 equivalent.
func LegacyTryjobStatusV0(s tryjob.Status) apiv0pb.Tryjob_Status {
	return apiv0pb.Tryjob_Status(s)
}

// LegacyTryjobResultStatusV0 converts internal Tryjob Result status to an APIv0 equivalent.
func LegacyTryjobResultStatusV0(s tryjob.Result_Status) apiv0pb.Tryjob_Result_Status {
	return apiv0pb.Tryjob_Result_Status(s)
}

// RunStatusV0 converts internal Run status to an APIv0 equivalent.
func RunStatusV0(s run.Status) apiv0pb.Run_Status {
	return apiv0pb.Run_Status(s)
}

// RunStatusV1 converts internal Run status to an APIv1 equivalent.
func RunStatusV1(s run.Status) apiv1pb.Run_Status {
	return apiv1pb.Run_Status(s)
}
