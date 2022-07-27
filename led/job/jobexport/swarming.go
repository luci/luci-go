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
	apipb "go.chromium.org/luci/swarming/proto/api"
)

var (
	// In order to have swarming service to upload output to RBE-CAS when no inputs.
	dummyCasDigest = &swarming.SwarmingRpcsDigest{
		Hash:            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SizeBytes:       0,
		ForceSendFields: []string{"SizeBytes"},
	}
)

// ToSwarmingNewTask renders a swarming proto task to a
// SwarmingRpcsNewTaskRequest.
// TODO(crbug.com/1345722): remove casUserPayload from argument list after
// led operates on Build instead of Buildbucket.
func ToSwarmingNewTask(sw *job.Swarming, casUserPayload *apipb.CASReference) (*swarming.SwarmingRpcsNewTaskRequest, error) {
	task := sw.Task
	if sw.GetCasUserPayload() != nil && sw.GetCasUserPayload().GetDigest() != nil {
		casUserPayload = sw.CasUserPayload
	}
	ret := &swarming.SwarmingRpcsNewTaskRequest{
		BotPingToleranceSecs: task.GetBotPingTolerance().GetSeconds(),
		Name:                 task.Name,
		User:                 task.User,
		ParentTaskId:         task.ParentTaskId,
		Priority:             int64(task.Priority),
		ServiceAccount:       task.ServiceAccount,
		Realm:                task.Realm,
		Tags:                 task.Tags,
		TaskSlices:           make([]*swarming.SwarmingRpcsTaskSlice, 0, len(task.TaskSlices)),
	}
	if rdbEnabled := task.GetResultdb().GetEnable(); rdbEnabled {
		ret.Resultdb = &swarming.SwarmingRpcsResultDBCfg{
			Enable: true,
		}
	}

	cupDigest := casUserPayload.GetDigest()

	for i, slice := range task.TaskSlices {
		props := slice.Properties

		slcCasDgst := props.GetCasInputRoot().GetDigest()
		// validate all isolate and rbe-cas related fields.
		if slcCasDgst != nil && cupDigest != nil &&
			(slcCasDgst.Hash != cupDigest.Hash || slcCasDgst.SizeBytes != cupDigest.SizeBytes) {
			return nil, errors.Reason(
				"slice %d defines CasInputRoot, but job.CasUserPayload is also defined. "+
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
				RelativeCwd: props.RelativeCwd,
			},
		}

		if con := props.GetContainment(); con.GetContainmentType() != apipb.Containment_NOT_SPECIFIED {
			toAdd.Properties.Containment = &swarming.SwarmingRpcsContainment{
				ContainmentType: con.GetContainmentType().String(),
			}
		}

		// If we have rbe-cas digest info, use that info.
		// Otherwise,  populate a dummy rbe-cas prop.
		//
		// The digest info in the slice will be used first. If it's not there, then
		// fall back to use the info in job-global "CasUserPayload"
		//
		// (The twisted logic will look a little bit better, after completely getting rid of isolate.)

		var casToUse *apipb.CASReference
		sliceCas := props.CasInputRoot
		jobCas := casUserPayload
		switch {
		case sliceCas.GetDigest().GetHash() != "":
			casToUse = sliceCas
		case jobCas.GetDigest().GetHash() != "":
			casToUse = jobCas
		case sliceCas.GetCasInstance() != "":
			casToUse = sliceCas
		default:
			casToUse = jobCas
		}

		if casToUse != nil {
			toAdd.Properties.CasInputRoot = &swarming.SwarmingRpcsCASReference{
				CasInstance: casToUse.CasInstance,
				Digest: &swarming.SwarmingRpcsDigest{
					Hash:            casToUse.Digest.GetHash(),
					SizeBytes:       casToUse.Digest.GetSizeBytes(),
					ForceSendFields: []string{"SizeBytes"}, // in case SizeBytes value is 0.
				},
			}
		} else {
			// populate a dummy CasInputRoot in order to use RBE-CAS.
			casIns, err := job.ToCasInstance(sw.Hostname)
			if err != nil {
				return nil, err
			}
			toAdd.Properties.CasInputRoot = &swarming.SwarmingRpcsCASReference{
				CasInstance: casIns,
				Digest:      dummyCasDigest,
			}
		}
		if toAdd.Properties.CasInputRoot.Digest.Hash == "" {
			toAdd.Properties.CasInputRoot.Digest = dummyCasDigest
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
