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
)

// RunStatusV0 converts internal CV status to public V0 status.
func RunStatusV0(s run.Status) apiv0pb.Run_Status {
	return apiv0pb.Run_Status(s)
}

// RunStatusV1 converts internal CV status to public V1 status.
func RunStatusV1(s run.Status) apiv1pb.Run_Status {
	return apiv1pb.Run_Status(s)
}
