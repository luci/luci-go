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
	"net/http"
	"sort"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job"
)

// Returns "bbagent", "kitchen" or "raw" depending on the type of task detected.
func detectMode(r *swarmingpb.NewTaskRequest) string {
	arg0, ts := "", r.TaskSlices[0]
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

// setPriority mutates the provided build to set the priority of its underlying
// swarming task.
//
// The priority for buildbucket type tasks is between 20 to 255.
func setPriority(build *bbpb.Build, priorityDiff int) {
	calPriority := func(originalPriority int32) int32 {
		switch priority := originalPriority + int32(priorityDiff); {
		case priority < 20:
			return 20
		case priority > 255:
			return 255
		default:
			return priority
		}
	}

	if build.Infra.Swarming != nil {
		build.Infra.Swarming.Priority = calPriority(build.Infra.Swarming.Priority)
	} else {
		config := build.Infra.Backend.GetConfig().GetFields()
		newPriority := calPriority(int32(config["priority"].GetNumberValue()))
		build.Infra.Backend.Config.Fields["priority"] = structpb.NewNumberValue(float64(newPriority))
	}
}

// FromNewTaskRequest generates a new job.Definition by parsing the
// given NewTaskRequest.
//
// If the task's first slice looks like either a bbagent or kitchen-based
// Buildbucket task, returns an error; otherwise the `swarming` field will be populated.
func FromNewTaskRequest(ctx context.Context, r *swarmingpb.NewTaskRequest, name, swarmingHost string, ks job.KitchenSupport, priorityDiff int, bld *bbpb.Build, extraTags []string, authClient *http.Client) (ret *job.Definition, err error) {
	if len(r.TaskSlices) == 0 {
		return nil, errors.New("swarming tasks without task slices are not supported")
	}

	ret = &job.Definition{}
	name = "led: " + name

	switch detectMode(r) {
	case "bbagent", "kitchen":
		return nil, errors.New(`"led get-swarm" has stopped supporting Buildbucket builds. Use "led get-builder" or "led get-build" instead.`)

	case "raw":
		// non-Buildbucket Swarming task
		sw := &job.Swarming{Hostname: swarmingHost}
		ret.JobType = &job.Definition_Swarming{Swarming: sw}
		jobDefinitionFromSwarming(sw, r)
		sw.Task.Name = name

	default:
		panic("impossible")
	}

	// ensure isolate/rbe-cas source consistency
	casUserPayload := &swarmingpb.CASReference{
		Digest: &swarmingpb.Digest{},
	}
	for i, slice := range r.TaskSlices {
		if cir := slice.Properties.CasInputRoot; cir != nil {
			if err := populateCasPayload(casUserPayload, cir); err != nil {
				return nil, errors.Annotate(err, "task slice %d", i).Err()
			}
		}
	}
	if casUserPayload.Digest.GetHash() == "" {
		return ret, err
	}

	if ret.GetSwarming() != nil {
		ret.GetSwarming().CasUserPayload = casUserPayload
	}

	return ret, err
}

func populateCasPayload(cas *swarmingpb.CASReference, cir *swarmingpb.CASReference) error {
	if cas.CasInstance == "" {
		cas.CasInstance = cir.CasInstance
	} else if cas.CasInstance != cir.CasInstance {
		return errors.Reason("RBE-CAS instance inconsistency: %q != %q", cas.CasInstance, cir.CasInstance).Err()
	}

	if cas.Digest.Hash != "" && (cir.Digest == nil || cir.Digest.Hash != cas.Digest.Hash) {
		return errors.Reason("RBE-CAS digest hash inconsistency: %+v != %+v", cas.Digest, cir.Digest).Err()
	} else if cir.Digest != nil {
		cas.Digest.Hash = cir.Digest.Hash
	}

	if cas.Digest.SizeBytes != 0 && (cir.Digest == nil || cir.Digest.SizeBytes != cas.Digest.SizeBytes) {
		return errors.Reason("RBE-CAS digest size bytes inconsistency: %+v != %+v", cas.Digest, cir.Digest).Err()
	} else if cir.Digest != nil {
		cas.Digest.SizeBytes = cir.Digest.SizeBytes
	}

	return nil
}

// FromBuild generates a new job.Definition using the provided Build.
func FromBuild(build *bbpb.Build, hostname, name string, priorityDiff int, extraTags []string) *job.Definition {
	ret := &job.Definition{}

	setPriority(build, priorityDiff)

	// Attach tags.
	tags := build.Tags
	tags = append(tags, &bbpb.StringPair{
		Key:   "led-job-name",
		Value: name,
	})
	tags = append(tags, &bbpb.StringPair{
		Key:   "user_agent",
		Value: "led",
	})
	for _, tag := range extraTags {
		k, v := strpair.Parse(tag)
		tags = append(tags, &bbpb.StringPair{
			Key:   k,
			Value: v,
		})
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Key < tags[j].Key })
	build.Tags = tags

	// Set buildbucket hostname.
	if build.Infra.Buildbucket.Hostname == "" {
		build.Infra.Buildbucket.Hostname = hostname
	}

	// Set build to be experimental.
	build.Input.Experimental = true // Legacy field, set it for now.
	enabled := stringset.NewFromSlice(build.Input.Experiments...)
	enabled.Add(buildbucket.ExperimentNonProduction)
	build.Input.Experiments = enabled.ToSortedSlice()

	build.Infra.Buildbucket.ExperimentReasons[buildbucket.ExperimentNonProduction] = bbpb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
	ret.JobType = &job.Definition_Buildbucket{
		Buildbucket: &job.Buildbucket{
			Name:                name,
			FinalBuildProtoPath: "build.proto.json",
			BbagentArgs: &bbpb.BBAgentArgs{
				Build: build,
			},
			RealBuild: true,
		},
	}

	return ret
}
