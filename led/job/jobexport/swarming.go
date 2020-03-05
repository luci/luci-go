// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package jobexport

import (
	"context"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
)

// ToSwarmingNewTask renders a (swarming) Definition to
// a SwarmingRpcsNewTaskRequest.
//
// If you call this on something other than a swarming Definition, it will
// panic.
func ToSwarmingNewTask(ctx context.Context, jd *job.Definition, uid string, ks job.KitchenSupport) (*swarming.SwarmingRpcsNewTaskRequest, error) {
	if err := jd.FlattenToSwarming(ctx, uid, ks); err != nil {
		return nil, errors.Annotate(err, "flattening to swarming Definition").Err()
	}

	task := jd.GetSwarming().Task
	ret := &swarming.SwarmingRpcsNewTaskRequest{
		BotPingToleranceSecs: task.GetBotPingTolerance().GetSeconds(),
		Name:                 task.Name,
		User:                 uid,
		Priority:             int64(task.Priority),
		Tags:                 task.Tags,
		TaskSlices:           make([]*swarming.SwarmingRpcsTaskSlice, 0, len(task.TaskSlices)),
	}

	for _, slice := range task.TaskSlices {
		props := slice.Properties

		toAdd := &swarming.SwarmingRpcsTaskSlice{
			ExpirationSecs:  slice.Expiration.Seconds,
			WaitForCapacity: slice.WaitForCapacity,
			Properties: &swarming.SwarmingRpcsTaskProperties{
				Caches: make([]*swarming.SwarmingRpcsCacheEntry, 0, len(props.NamedCaches)),

				Dimensions: make([]*swarming.SwarmingRpcsStringPair, 0, len(props.Dimensions)),

				ExecutionTimeoutSecs: props.GetExecutionTimeout().GetSeconds(),
				GracePeriodSecs:      props.GetGracePeriod().GetSeconds(),
				IoTimeoutSecs:        props.GetIoTimeout().GetSeconds(),

				InputsRef: &swarming.SwarmingRpcsFilesRef{
					Isolated:       props.GetCasInputs().GetDigest(),
					Isolatedserver: props.GetCasInputs().GetServer(),
					Namespace:      props.GetCasInputs().GetNamespace(),
				},
				CipdInput: &swarming.SwarmingRpcsCipdInput{
					Packages: make([]*swarming.SwarmingRpcsCipdPackage, 0, len(props.CipdInputs)),
				},

				Env:         make([]*swarming.SwarmingRpcsStringPair, 0, len(props.Env)),
				EnvPrefixes: make([]*swarming.SwarmingRpcsStringListPair, 0, len(props.EnvPaths)),

				Command:     props.Command,
				ExtraArgs:   props.ExtraArgs,
				RelativeCwd: props.RelativeCwd,

				Containment: &swarming.SwarmingRpcsContainment{
					ContainmentType:           props.Containment.ContainmentType.String(),
					LimitProcesses:            props.Containment.LimitProcesses,
					LimitTotalCommittedMemory: props.Containment.LimitTotalCommittedMemory,
					LowerPriority:             props.Containment.LowerPriority,
				},
			},
		}

		for _, env := range props.Env {
			toAdd.Properties.Env = append(toAdd.Properties.Env, &swarming.SwarmingRpcsStringPair{
				Key:   env.Key,
				Value: env.Value,
			})
		}

		for _, path := range props.EnvPaths {
			toAdd.Properties.EnvPrefixes = append(toAdd.Properties.EnvPrefixes, &swarming.SwarmingRpcsStringListPair{
				Key:   path.Key,
				Value: path.Values,
			})
		}

		for _, cache := range props.NamedCaches {
			toAdd.Properties.Caches = append(toAdd.Properties.Caches, &swarming.SwarmingRpcsCacheEntry{
				Name: cache.Name,
				Path: cache.DestPath,
			})
		}

		for _, pkg := range props.CipdInputs {
			toAdd.Properties.CipdInput.Packages = append(toAdd.Properties.CipdInput.Packages, &swarming.SwarmingRpcsCipdPackage{
				PackageName: pkg.PackageName,
				Version:     pkg.Version,
				Path:        pkg.DestPath,
			})
		}

		for _, dim := range props.Dimensions {
			for _, val := range dim.Values {
				toAdd.Properties.Dimensions = append(toAdd.Properties.Dimensions, &swarming.SwarmingRpcsStringPair{
					Key:   dim.Key,
					Value: val,
				})
			}
		}

		ret.TaskSlices = append(ret.TaskSlices, toAdd)
	}

	return ret, nil
}
