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

package ledcmd

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/job/jobcreate"
)

// GetBuildOpts are the options for GetBuild.
type GetBuildOpts struct {
	BuildbucketHost string
	BuildID         int64
	PinBotID        bool
	PriorityDiff    int
	KitchenSupport  job.KitchenSupport
	RealBuild       bool
	Experiments     map[string]bool
}

func getBuildJobName(opts GetBuildOpts) string {
	return fmt.Sprintf("get-build %d", opts.BuildID)
}

// GetBuild retrieves a job Definition from a Buildbucket build.
func GetBuild(ctx context.Context, authClient *http.Client, opts GetBuildOpts) (*job.Definition, error) {
	logging.Infof(ctx, "getting build definition")

	jd, synErr := synthesizeBuildFromTemplate(ctx, authClient, opts)
	if opts.RealBuild {
		return jd, synErr
	}

	bbClient := newBuildbucketClient(authClient, opts.BuildbucketHost)

	answer, err := bbClient.GetBuild(ctx, &bbpb.GetBuildRequest{
		Id: opts.BuildID,
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

	jdBucket := jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetBuilder().GetBucket()
	if synErr == nil && jdBucket != answer.GetBuilder().GetBucket() {
		// The builder has shadow bucket enabled, switch to real build mode automatically.
		logging.Infof(ctx, "Bucket %s has shadow bucket %s, Switching to led real-build automatically.", answer.GetBuilder().GetBucket(), jdBucket)
		return jd, err
	}

	if answer.Infra.GetBuildbucket().GetAgent().GetOutput() != nil {
		answer.Infra.Buildbucket.Agent.Output = nil
	}

	logging.Infof(ctx, "getting build definition: done")

	swarmingTaskID := answer.Infra.Swarming.GetTaskId()
	swarmingHostname := answer.Infra.Swarming.GetHostname()

	if swarmingTaskID == "" {
		return nil, errors.New("unable to find swarming task ID on buildbucket task")
	}
	if swarmingHostname == "" {
		return nil, errors.New("unable to find swarming hostname on buildbucket task")
	}

	return GetFromSwarmingTask(ctx, authClient, answer, GetFromSwarmingTaskOpts{
		SwarmingHost:   swarmingHostname,
		TaskID:         swarmingTaskID,
		PinBotID:       opts.PinBotID,
		Name:           getBuildJobName(opts),
		PriorityDiff:   opts.PriorityDiff,
		KitchenSupport: opts.KitchenSupport,
	})
}

func synthesizeBuildFromTemplate(ctx context.Context, authClient *http.Client, opts GetBuildOpts) (*job.Definition, error) {
	bbClient := newBuildbucketClient(authClient, opts.BuildbucketHost)
	build, err := bbClient.SynthesizeBuild(ctx, &bbpb.SynthesizeBuildRequest{
		TemplateBuildId: opts.BuildID,
		Experiments:     opts.Experiments,
	})
	if err != nil {
		return nil, err
	}
	return jobcreate.FromBuild(build, opts.BuildbucketHost, getBuildJobName(opts), opts.PriorityDiff, nil), nil
}
