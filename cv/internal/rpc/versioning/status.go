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
	apiv0pb "go.chromium.org/luci/cv/api/v0"
	apiv1pb "go.chromium.org/luci/cv/api/v1"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// TryjobStatusV0 converts internal Tryjob status to an APIv0 equivalent.
func TryjobStatusV0(s tryjob.Status) apiv0pb.Tryjob_Status {
	return apiv0pb.Tryjob_Status(s)
}

// RunStatusV0 converts internal Run status to an APIv0 equivalent.
func RunStatusV0(s run.Status) apiv0pb.Run_Status {
	return apiv0pb.Run_Status(s)
}

// RunStatusV1 converts internal Run status to an APIv1 equivalent.
func RunStatusV1(s run.Status) apiv1pb.Run_Status {
	return apiv1pb.Run_Status(s)
}
