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

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/led/job"
	"google.golang.org/genproto/protobuf/field_mask"
)

// GetBuildOpts are the options for GetBuild.
type GetBuildOpts struct {
	BuildbucketHost string
	BuildID         int64
	PinBotID        bool
	PriorityDiff    int
	KitchenSupport  job.KitchenSupport
}

// GetBuild retrieves a job Definition from a Buildbucket build.
func GetBuild(ctx context.Context, authClient *http.Client, opts GetBuildOpts) (*job.Definition, error) {
	logging.Infof(ctx, "getting build definition")
	bbucket := newBuildbucketClient(authClient, opts.BuildbucketHost)

	answer, err := bbucket.GetBuild(ctx, &bbpb.GetBuildRequest{
		Id: opts.BuildID,
		Fields: &field_mask.FieldMask{
			Paths: []string{"infra"},
		},
	})
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "getting build definition: done")

	swarmingTaskID := answer.Infra.Swarming.TaskId
	swarmingHostname := answer.Infra.Swarming.Hostname

	if swarmingTaskID == "" {
		return nil, errors.New("unable to find swarming task ID on buildbucket task")
	}
	if swarmingHostname == "" {
		return nil, errors.New("unable to find swarming hostname on buildbucket task")
	}

	return GetFromSwarmingTask(ctx, authClient, GetFromSwarmingTaskOpts{
		SwarmingHost:   swarmingHostname,
		TaskID:         swarmingTaskID,
		PinBotID:       opts.PinBotID,
		Name:           fmt.Sprintf("get-build %d", opts.BuildID),
		PriorityDiff:   opts.PriorityDiff,
		KitchenSupport: opts.KitchenSupport,
	})
}
