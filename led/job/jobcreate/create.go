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
	"context"
	"path"

	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

// Returns "bbagent", "kitchen" or "raw" depending on the type of task detected.
func detectMode(r *swarming.SwarmingRpcsNewTaskRequest) string {
	arg0, ts := "", &swarming.SwarmingRpcsTaskSlice{}
	ts = r.TaskSlices[0]
	if ts.Properties != nil {
		if len(ts.Properties.Command) > 0 {
			arg0 = ts.Properties.Command[0]
		}
	}
	switch arg0 {
	case "bbagent${EXECUTABLE_SUFFIX}":
		return "bbagent"
	case "kitchen${EXECUTABLE_SUFFIX}":
		return "kitchen"
	}
	return "raw"
}

// FromNewTaskRequest generates a new job.Definition by parsing the
// given SwarmingRpcsNewTaskRequest.
//
// If the task's first slice looks like either a bbagent or kitchen-based
// Buildbucket task, the returned Definition will have the `buildbucket`
// field populated, otherwise the `swarming` field will be populated.
func FromNewTaskRequest(ctx context.Context, r *swarming.SwarmingRpcsNewTaskRequest, name, swarmingHost string, ks job.KitchenSupport) (ret *job.Definition, err error) {
	if len(r.TaskSlices) == 0 {
		return nil, errors.New("swarming tasks without task slices are not supported")
	}

	ret = &job.Definition{UserPayload: &swarmingpb.CASTree{}}
	name = "led: " + name

	switch detectMode(r) {
	case "bbagent":
		bb := &job.Buildbucket{Name: name}
		ret.JobType = &job.Definition_Buildbucket{Buildbucket: bb}
		bbCommonFromTaskRequest(bb, r)
		bb.BbagentArgs, err = bbinput.Parse(r.TaskSlices[0].Properties.Command[1])

	case "kitchen":
		bb := &job.Buildbucket{LegacyKitchen: true, Name: name}
		ret.JobType = &job.Definition_Buildbucket{Buildbucket: bb}
		bbCommonFromTaskRequest(bb, r)
		err = ks.FromSwarming(ctx, r, bb)

	case "raw":
		// non-Buildbucket Swarming task
		sw := &job.Swarming{Hostname: swarmingHost}
		ret.JobType = &job.Definition_Swarming{Swarming: sw}
		jobDefinitionFromSwarming(sw, r)
		sw.Task.Name = name

	default:
		panic("impossible")
	}

	if bb := ret.GetBuildbucket(); err == nil && bb != nil {
		bb.BbagentArgs.Build.Id = 0
		bb.BbagentArgs.Build.Infra.Logdog.Prefix = ""
		bb.BbagentArgs.Build.Infra.Swarming.TaskId = ""

		// set all buildbucket type tasks to experimental by default.
		bb.BbagentArgs.Build.Input.Experimental = true

		// bump priority by default
		bb.BbagentArgs.Build.Infra.Swarming.Priority += 10

		// clear fields which don't make sense
		bb.BbagentArgs.Build.CreatedBy = ""
		bb.BbagentArgs.Build.CanceledBy = ""
		bb.BbagentArgs.Build.CreateTime = nil
		bb.BbagentArgs.Build.UpdateTime = nil
		bb.BbagentArgs.Build.Id = 0
		bb.BbagentArgs.Build.Number = 0
		bb.BbagentArgs.Build.Infra.Buildbucket.RequestedProperties = nil
		bb.BbagentArgs.Build.Status = 0

		// drop legacy recipe fields
		if recipe := bb.BbagentArgs.Build.Infra.Recipe; recipe != nil {
			dropRecipePackage(&bb.CipdPackages, path.Dir(bb.BbagentArgs.ExecutablePath))
			bb.BbagentArgs.Build.Infra.Recipe = nil
		}
	}

	// ensure isolate source consistency
	for i, slice := range r.TaskSlices {
		ir := slice.Properties.InputsRef
		if ir == nil {
			continue
		}

		if ret.UserPayload.Server == "" {
			ret.UserPayload.Server = ir.Isolatedserver
		} else if ret.UserPayload.Server != ir.Isolatedserver {
			return nil, errors.Reason("isolate server inconsistency in slice %d: %q != %q",
				i, ret.UserPayload.Server, ir.Isolatedserver).Err()
		}

		if ret.UserPayload.Namespace == "" {
			ret.UserPayload.Namespace = ir.Namespace
		} else if ret.UserPayload.Namespace != ir.Namespace {
			return nil, errors.Reason("isolate namespace inconsistency in slice %d: %q != %q",
				i, ret.UserPayload.Namespace, ir.Namespace).Err()
		}
	}

	return ret, err
}
