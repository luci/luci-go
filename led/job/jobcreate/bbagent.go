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

package jobcreate

import (
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/led/job"
)

func bbCommonFromTaskRequest(bb *job.Buildbucket, r *swarming.SwarmingRpcsNewTaskRequest) {
	ts := r.TaskSlices[0]

	bb.EnsureBasics()

	bb.CipdPackages = cipdPins(ts.Properties.CipdInput)
	bb.EnvVars = strPairs(ts.Properties.Env, func(key string) bool {
		if key == "BUILDBUCKET_EXPERIMENTAL" {
			return false
		}
		return true
	})
	bb.EnvPrefixes = strListPairs(ts.Properties.EnvPrefixes)

	bb.BbagentArgs.Build.SchedulingTimeout = durationpb.New(
		time.Second * time.Duration(r.ExpirationSecs))
	bb.BotPingTolerance = durationpb.New(
		time.Second * time.Duration(r.BotPingToleranceSecs))

	bb.Containment = containmentFromSwarming(ts.Properties.Containment)
}
