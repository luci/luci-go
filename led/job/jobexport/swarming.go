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

	for i, slice := range task.TaskSlices {
		props := slice.Properties

		if props.GetCasInputs().GetDigest() != "" && jd.GetUserPayload().GetDigest() != "" {
			return nil, errors.Reason(
				"slice %d defines CasInputs, but job.UserPayload is also defined. "+
					"Call ConsolidateIsolateds before calling ToSwarmingNewTask.", i).Err()
		}

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
					ContainmentType:           props.GetContainment().GetContainmentType().String(),
					LimitProcesses:            props.GetContainment().GetLimitProcesses(),
					LimitTotalCommittedMemory: props.GetContainment().GetLimitTotalCommittedMemory(),
					LowerPriority:             props.GetContainment().GetLowerPriority(),
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
