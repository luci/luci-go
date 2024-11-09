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
	"path"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
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
// Buildbucket task, the returned Definition will have the `buildbucket`
// field populated, otherwise the `swarming` field will be populated.
func FromNewTaskRequest(ctx context.Context, r *swarmingpb.NewTaskRequest, name, swarmingHost string, ks job.KitchenSupport, priorityDiff int, bld *bbpb.Build, extraTags []string, authClient *http.Client) (ret *job.Definition, err error) {
	if len(r.TaskSlices) == 0 {
		return nil, errors.New("swarming tasks without task slices are not supported")
	}

	ret = &job.Definition{}
	name = "led: " + name

	switch detectMode(r) {
	case "bbagent":
		bb := &job.Buildbucket{}
		ret.JobType = &job.Definition_Buildbucket{Buildbucket: bb}
		// TODO(crbug.com/1219018): use bbCommonFromTaskRequest only in the long
		// bbagent arg case.
		// Discussion: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/3511002/comments/0daf496b_2c8ba5a2
		bbCommonFromTaskRequest(bb, r)
		cmd := r.TaskSlices[0].Properties.Command
		switch {
		case len(cmd) == 2:
			bb.BbagentArgs, err = bbinput.Parse(cmd[len(cmd)-1])
			bb.UpdateBuildFromBbagentArgs()
		case bld != nil:
			bb.BbagentArgs = bbagentArgsFromBuild(bld)
		default:
			bb.BbagentArgs, err = getBbagentArgsFromCMD(ctx, cmd, authClient)
			bb.UpdateBuildFromBbagentArgs()
		}

		// This check is only here because of bbCommonFromTaskRequest.
		// TODO(crbug.com/1219018): remove this check after bbCommonFromTaskRequest
		// is only used for long bbagent arg case.
		if bb.BbagentDownloadCIPDPkgs() {
			bb.CipdPackages = nil
		}

	case "kitchen":
		bb := &job.Buildbucket{LegacyKitchen: true}
		ret.JobType = &job.Definition_Buildbucket{Buildbucket: bb}
		bbCommonFromTaskRequest(bb, r)
		err = ks.FromSwarmingV2(ctx, r, bb)

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
		bb.Name = name
		bb.FinalBuildProtoPath = "build.proto.json"

		// set all buildbucket type tasks to experimental by default.
		bb.BbagentArgs.Build.Input.Experimental = true

		setPriority(bb.BbagentArgs.Build, priorityDiff)

		// clear fields which don't make sense
		bb.BbagentArgs.Build.CanceledBy = ""
		bb.BbagentArgs.Build.CreatedBy = ""
		bb.BbagentArgs.Build.CreateTime = nil
		bb.BbagentArgs.Build.Id = 0
		bb.BbagentArgs.Build.Infra.Buildbucket.Hostname = ""
		bb.BbagentArgs.Build.Infra.Buildbucket.RequestedProperties = nil
		bb.BbagentArgs.Build.Infra.Logdog.Prefix = ""
		bb.BbagentArgs.Build.Infra.Swarming.TaskId = ""
		bb.BbagentArgs.Build.Number = 0
		bb.BbagentArgs.Build.Status = 0
		bb.BbagentArgs.Build.UpdateTime = nil

		bb.BbagentArgs.Build.Tags = nil
		if len(extraTags) > 0 {
			tags := make([]*bbpb.StringPair, 0, len(extraTags))
			for _, tag := range extraTags {
				k, v := strpair.Parse(tag)
				tags = append(tags, &bbpb.StringPair{
					Key:   k,
					Value: v,
				})
			}
			sort.Slice(tags, func(i, j int) bool { return tags[i].Key < tags[j].Key })
			bb.BbagentArgs.Build.Tags = tags
		}

		// drop the executable path; it's canonically represented by
		// out.BBAgentArgs.PayloadPath and out.BBAgentArgs.Build.Exe.
		if exePath := bb.BbagentArgs.ExecutablePath; exePath != "" {
			// convert to new mode
			payload, arg := path.Split(exePath)
			bb.BbagentArgs.ExecutablePath = ""
			bb.UpdatePayloadPath(strings.TrimSuffix(payload, "/"))
			bb.BbagentArgs.Build.Exe.Cmd = []string{arg}
		}

		if !bb.BbagentDownloadCIPDPkgs() {
			dropRecipePackage(&bb.CipdPackages, bb.PayloadPath())
		}

		props := bb.BbagentArgs.GetBuild().GetInput().GetProperties()
		// everything in here is reflected elsewhere in the Build and will be
		// re-synthesized by kitchen support or the recipe engine itself, depending
		// on the final kitchen/bbagent execution mode.
		delete(props.GetFields(), "$recipe_engine/runtime")

		// drop legacy recipe fields
		if recipe := bb.BbagentArgs.Build.Infra.Recipe; recipe != nil {
			bb.BbagentArgs.Build.Infra.Recipe = nil
		}
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
	if ret.GetBuildbucket() != nil {
		// `led get-builder` is still using swarmingbucket.get_task_def, so
		// we need to fill in the data to ret.GetBuildbucket() for its builds.
		// TODO(crbug.com/1345722): remove this after we migrate away from
		// swarmingbucket.get_task_def.
		payloadPath := ret.GetBuildbucket().BbagentArgs.PayloadPath
		updates := &bbpb.BuildInfra_Buildbucket_Agent{
			Input: &bbpb.BuildInfra_Buildbucket_Agent_Input{
				Data: map[string]*bbpb.InputDataRef{
					payloadPath: {
						DataType: &bbpb.InputDataRef_Cas{
							Cas: &bbpb.InputDataRef_CAS{
								CasInstance: casUserPayload.GetCasInstance(),
								Digest: &bbpb.InputDataRef_CAS_Digest{
									Hash:      casUserPayload.GetDigest().GetHash(),
									SizeBytes: casUserPayload.GetDigest().GetSizeBytes(),
								},
							},
						},
					},
				},
			},
			Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
				payloadPath: bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
			},
		}
		ret.GetBuildbucket().UpdateBuildbucketAgent(updates)
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

func getBbagentArgsFromCMD(ctx context.Context, cmd []string, authClient *http.Client) (*bbpb.BBAgentArgs, error) {
	var hostname string
	var bID int64
	for i, s := range cmd {
		switch {
		case s == "-host" && i < len(cmd)-1:
			hostname = cmd[i+1]
		case s == "-build-id" && i < len(cmd)-1:
			var err error
			if bID, err = strconv.ParseInt(cmd[i+1], 10, 64); err != nil {
				return nil, errors.Annotate(err, "cmd -build-id").Err()
			}
		}
	}
	if hostname == "" && bID == 0 {
		// This could happen if the cmd was for a led build like
		// `bbagent${EXECUTABLE_SUFFIX} --output ${ISOLATED_OUTDIR}/build.proto.json <encoded bbinput>`
		return bbinput.Parse(cmd[len(cmd)-1])
	}
	if hostname == "" {
		return nil, errors.New("host is required in cmd")
	}
	if bID == 0 {
		return nil, errors.New("build-id is required in cmd")
	}
	bbclient := bbpb.NewBuildsPRPCClient(&prpc.Client{
		C:    authClient,
		Host: hostname,
	})
	bld, err := bbclient.GetBuild(ctx, &bbpb.GetBuildRequest{
		Id: bID,
		Mask: &bbpb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{
					"builder",
					"infra",
					"input",
					"scheduling_timeout",
					"execution_timeout",
					"grace_period",
					"exe",
					"tags",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return bbagentArgsFromBuild(bld), nil
}

// TODO(crbug.com/1098551): Invert this and make led use the build proto directly.
func bbagentArgsFromBuild(bld *bbpb.Build) *bbpb.BBAgentArgs {
	return &bbpb.BBAgentArgs{
		PayloadPath:            bld.Infra.Bbagent.PayloadPath,
		CacheDir:               bld.Infra.Bbagent.CacheDir,
		KnownPublicGerritHosts: bld.Infra.Buildbucket.KnownPublicGerritHosts,
		Build:                  bld,
	}
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
