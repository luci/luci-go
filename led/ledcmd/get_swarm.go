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
	"net/http"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"

	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/job/jobcreate"
)

// GetFromSwarmingTaskOpts are the options for GetFromSwarmingTask.
type GetFromSwarmingTaskOpts struct {
	// The swarming host to retrieve the task from.
	SwarmingHost string

	// The ID of the task to retrieve.
	TaskID string

	// If the resulting Definition should be pinned to the same bot id that the
	// original task ran on (replaces job dimensions with just the 'id'
	// dimension).
	//
	// NOTE: This only "works" for bots which are managed statically. Dynamically
	// allocated bots (e.g. those from GCE Provider) have names which may recycle
	// from the time of the original swarming task to when GetFromSwarmingTask
	// runs, which means that 'pinning' will only get you a bot with the same
	// name, not necessarially the original bot.
	//
	// TODO: Remove this when we no longer mangage bots statically.
	PinBotID bool

	// The "name" of the resulting job Definition.
	Name string

	// The difference from the original priority.
	PriorityDiff int

	KitchenSupport job.KitchenSupport
}

// GetFromSwarmingTask retrieves and renders a JobDefinition from the given
// swarming task, printing it to stdout and returning an error.
func GetFromSwarmingTask(ctx context.Context, authClient *http.Client, bld *bbpb.Build, opts GetFromSwarmingTaskOpts) (*job.Definition, error) {
	if opts.KitchenSupport == nil {
		opts.KitchenSupport = job.NoKitchenSupport()
	}

	logging.Infof(ctx, "getting task definition: %q %q", opts.SwarmingHost, opts.TaskID)
	swarm := newSwarmClient(authClient, opts.SwarmingHost)

	req, err := swarm.Task.Request(opts.TaskID).Do()
	if err != nil {
		return nil, err
	}

	jd, err := jobcreate.FromNewTaskRequest(
		ctx, taskRequestToNewTaskRequest(req), opts.Name,
		opts.SwarmingHost, opts.KitchenSupport, opts.PriorityDiff, bld, authClient)
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "getting task definition: done")

	if opts.PinBotID {
		logging.Infof(ctx, "pinning swarming bot id")

		rslt, err := swarm.Task.Result(opts.TaskID).Do()
		if err != nil {
			return nil, err
		}
		if len(rslt.BotDimensions) == 0 {
			return nil, errors.Reason("could not pin bot ID, task is %q", rslt.State).Err()
		}

		id := ""
		for _, d := range rslt.BotDimensions {
			if d.Key == "id" {
				id = d.Value[0]
				break
			}
		}

		if id == "" {
			return nil, errors.New("could not pin bot ID (bot ID not found)")
		}

		pool := ""

	poolfind:
		for _, slc := range req.TaskSlices {
			if slc.Properties == nil {
				continue
			}

			for _, dim := range slc.Properties.Dimensions {
				if dim.Key == "pool" {
					pool = dim.Value
					break poolfind
				}
			}
		}

		if pool == "" {
			return nil, errors.New("could not pin bot ID (task dimension 'pool' not found)")
		}

		err = jd.Edit(func(je job.Editor) {
			je.SetDimensions(map[string][]job.ExpiringValue{
				"pool": {{Value: pool}},
				"id":   {{Value: id}},
			})
		})
		if err != nil {
			return nil, err
		}
	}

	if err := fillCasDefaults(jd); err != nil {
		return nil, err
	}
	return jd, nil
}

// fill the cas default if no isolate inputs.
func fillCasDefaults(jd *job.Definition) error {
	if jd.CasUserPayload.GetCasInstance() == "" {
		cas, err := jd.CasInstance()
		if err != nil {
			return err
		}
		jd.CasUserPayload = &swarmingpb.CASReference{
			CasInstance: cas,
		}
	}
	if jd.GetSwarming() != nil && jd.GetSwarming().GetCasUserPayload() == nil {
		jd.GetSwarming().CasUserPayload = jd.CasUserPayload
	}
	return nil
}

// swarming has two separate structs to represent a task request.
//
// Convert from 'TaskRequest' to 'NewTaskRequest'.
func taskRequestToNewTaskRequest(req *swarming.SwarmingRpcsTaskRequest) *swarming.SwarmingRpcsNewTaskRequest {
	return &swarming.SwarmingRpcsNewTaskRequest{
		Name:           req.Name,
		ExpirationSecs: req.ExpirationSecs,
		Priority:       req.Priority,
		Properties:     req.Properties,
		TaskSlices:     req.TaskSlices,
		// don't want these or some random person/service will get notified :
		//PubsubTopic:    req.PubsubTopic,
		//PubsubUserdata: req.PubsubUserdata,
		Tags:           req.Tags,
		User:           req.User,
		ServiceAccount: req.ServiceAccount,
		Realm:          req.Realm,
	}
}
