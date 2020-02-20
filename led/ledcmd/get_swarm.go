// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ledcmd

import (
	"context"
	"net/http"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/job/jobcreate"
)

// GetFromSwarmingTaskOpts are the options for GetFromSwarmingTask.
type GetFromSwarmingTaskOpts struct {
	// The swarming host to retrieve the task from.
	SwarmingHost string

	// The ID of the task to retrieve.
	TaskID string

	// If the resulting Definition should be pinned to the same bot that the
	// original task ran on (replaces job dimensions with just the 'id'
	// dimension).
	PinBot bool

	// The "name" of the resulting job Definition.
	Name string
}

// GetFromSwarmingTask retrieves and renders a JobDefinition from the given
// swarming task, printing it to stdout and returning an error.
func GetFromSwarmingTask(ctx context.Context, authClient *http.Client, opts GetFromSwarmingTaskOpts) (*job.Definition, error) {
	logging.Infof(ctx, "getting task definition: %q %q", opts.SwarmingHost, opts.TaskID)
	swarm := newSwarmClient(authClient, opts.SwarmingHost)

	req, err := swarm.Task.Request(opts.TaskID).Do()
	if err != nil {
		return nil, err
	}

	jd, err := jobcreate.FromNewTaskRequest(
		taskRequestToNewTaskRequest(req), opts.Name, opts.SwarmingHost)
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "getting task definition: done")

	if opts.PinBot {
		logging.Infof(ctx, "pinning swarming bot")

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
				"pool": []job.ExpiringValue{{Value: pool}},
				"id":   []job.ExpiringValue{{Value: id}},
			})
		})
		if err != nil {
			return nil, err
		}
	}

	return jd, nil
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
		// don't wan't these or some random person/service will get notified :
		//PubsubTopic:    req.PubsubTopic,
		//PubsubUserdata: req.PubsubUserdata,
		Tags:           req.Tags,
		User:           req.User,
		ServiceAccount: req.ServiceAccount,
	}
}
