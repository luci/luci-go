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
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/swarming/proto/api"
)

// ToSwarmingNewTask renders a swarming proto task to a
// SwarmingRpcsNewTaskRequest.
func ToSwarmingNewTask(sw *job.Swarming, userPayload *apipb.CASTree) (*swarming.SwarmingRpcsNewTaskRequest, error) {
	task := sw.Task
	ret := &swarming.SwarmingRpcsNewTaskRequest{
		BotPingToleranceSecs: task.GetBotPingTolerance().GetSeconds(),
		Name:                 task.Name,
		User:                 task.User,
		ParentTaskId:         task.ParentTaskId,
		Priority:             int64(task.Priority),
		ServiceAccount:       task.ServiceAccount,
		Tags:                 task.Tags,
		TaskSlices:           make([]*swarming.SwarmingRpcsTaskSlice, 0, len(task.TaskSlices)),
	}

	upDigest := userPayload.GetDigest()

	for i, slice := range task.TaskSlices {
		props := slice.Properties

		slcDgst := props.GetCasInputs().GetDigest()

		if slcDgst != "" && upDigest != "" && slcDgst != upDigest {
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

				CipdInput: &swarming.SwarmingRpcsCipdInput{
					Packages: make([]*swarming.SwarmingRpcsCipdPackage, 0, len(props.CipdInputs)),
				},

				Env:         make([]*swarming.SwarmingRpcsStringPair, 0, len(props.Env)),
				EnvPrefixes: make([]*swarming.SwarmingRpcsStringListPair, 0, len(props.EnvPaths)),

				Command:     props.Command,
				ExtraArgs:   props.ExtraArgs,
				RelativeCwd: props.RelativeCwd,
			},
		}

		if con := props.GetContainment(); con.GetContainmentType() != apipb.Containment_NOT_SPECIFIED {
			toAdd.Properties.Containment = &swarming.SwarmingRpcsContainment{
				ContainmentType:           con.GetContainmentType().String(),
				LimitProcesses:            con.GetLimitProcesses(),
				LimitTotalCommittedMemory: con.GetLimitTotalCommittedMemory(),
				LowerPriority:             con.GetLowerPriority(),
			}
		}

		// This is really awkward!
		//
		//   If the slice has a digest, then we always use all that info.
		//   Otherwise if the job-global "UserPayload" has a digest, then use all of
		//     that info.
		//   Otherwise, if the slice has just namespace/server info, then use that.
		//   Finally, fall back to the server info in UserPayload
		//
		// Isn't isolate so simple?
		var isoToUse *apipb.CASTree
		sliceIso := props.CasInputs
		jobIso := userPayload
		switch {
		case sliceIso.GetDigest() != "":
			isoToUse = sliceIso

		case jobIso.GetDigest() != "":
			isoToUse = jobIso

		case sliceIso.GetServer() != "" || sliceIso.GetNamespace() != "":
			isoToUse = sliceIso

		default:
			isoToUse = jobIso
		}
		if isoToUse != nil {
			toAdd.Properties.InputsRef = &swarming.SwarmingRpcsFilesRef{
				Isolated:       isoToUse.Digest,
				Isolatedserver: isoToUse.Server,
				Namespace:      isoToUse.Namespace,
			}
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
