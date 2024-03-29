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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

var (
	// In order to have swarming service to upload output to RBE-CAS when no inputs.
	dummyCasDigest = &swarmingpb.Digest{
		Hash:      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		SizeBytes: 0,
	}
)

// ToSwarmingNewTask renders a swarming proto task to a
// NewTaskRequest.
func ToSwarmingNewTask(sw *job.Swarming) (*swarmingpb.NewTaskRequest, error) {
	task := sw.Task
	ret := &swarmingpb.NewTaskRequest{
		BotPingToleranceSecs: task.GetBotPingToleranceSecs(),
		Name:                 task.Name,
		User:                 task.User,
		ParentTaskId:         task.ParentTaskId,
		Priority:             task.Priority,
		ServiceAccount:       task.ServiceAccount,
		Realm:                task.Realm,
		Tags:                 task.Tags,
		TaskSlices:           make([]*swarmingpb.TaskSlice, 0, len(task.TaskSlices)),
	}
	if rdbEnabled := task.GetResultdb().GetEnable(); rdbEnabled {
		ret.Resultdb = &swarmingpb.ResultDBCfg{
			Enable: true,
		}
	}

	casUserPayload := sw.GetCasUserPayload()
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

		toAdd := &swarmingpb.TaskSlice{
			ExpirationSecs:  slice.ExpirationSecs,
			WaitForCapacity: slice.WaitForCapacity,
			Properties: &swarmingpb.TaskProperties{
				Caches: make([]*swarmingpb.CacheEntry, 0, len(props.Caches)),

				Dimensions: make([]*swarmingpb.StringPair, 0, len(props.Dimensions)),

				ExecutionTimeoutSecs: props.GetExecutionTimeoutSecs(),
				GracePeriodSecs:      props.GetGracePeriodSecs(),
				IoTimeoutSecs:        props.GetIoTimeoutSecs(),

				CipdInput: &swarmingpb.CipdInput{
					Packages: make([]*swarmingpb.CipdPackage, 0, len(props.CipdInput.Packages)),
				},

				Env:         make([]*swarmingpb.StringPair, 0, len(props.Env)),
				EnvPrefixes: make([]*swarmingpb.StringListPair, 0, len(props.EnvPrefixes)),

				Command:     props.Command,
				RelativeCwd: props.RelativeCwd,
			},
		}

		if typ := props.GetContainment().GetContainmentType(); typ != swarmingpb.Containment_NOT_SPECIFIED {
			toAdd.Properties.Containment = &swarmingpb.Containment{
				ContainmentType: typ,
			}
		}

		// If we have rbe-cas digest info, use that info.
		// Otherwise,  populate a dummy rbe-cas prop.
		//
		// The digest info in the slice will be used first. If it's not there, then
		// fall back to use the info in job-global "CasUserPayload"
		//
		// (The twisted logic will look a little bit better, after completely getting rid of isolate.)

		var casToUse *swarmingpb.CASReference
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
			toAdd.Properties.CasInputRoot = &swarmingpb.CASReference{
				CasInstance: casToUse.CasInstance,
				Digest: &swarmingpb.Digest{
					Hash:      casToUse.Digest.GetHash(),
					SizeBytes: casToUse.Digest.GetSizeBytes(),
				},
			}
		} else {
			// populate a dummy CasInputRoot in order to use RBE-CAS.
			casIns, err := job.ToCasInstance(sw.Hostname)
			if err != nil {
				return nil, err
			}
			toAdd.Properties.CasInputRoot = &swarmingpb.CASReference{
				CasInstance: casIns,
				Digest:      dummyCasDigest,
			}
		}
		if toAdd.Properties.CasInputRoot.Digest.Hash == "" {
			toAdd.Properties.CasInputRoot.Digest = dummyCasDigest
		}

		for _, env := range props.Env {
			toAdd.Properties.Env = append(toAdd.Properties.Env, &swarmingpb.StringPair{
				Key:   env.Key,
				Value: env.Value,
			})
		}

		for _, path := range props.EnvPrefixes {
			toAdd.Properties.EnvPrefixes = append(toAdd.Properties.EnvPrefixes, &swarmingpb.StringListPair{
				Key:   path.Key,
				Value: path.Value,
			})
		}

		for _, cache := range props.Caches {
			toAdd.Properties.Caches = append(toAdd.Properties.Caches, &swarmingpb.CacheEntry{
				Name: cache.Name,
				Path: cache.Path,
			})
		}

		for _, pkg := range props.CipdInput.Packages {
			toAdd.Properties.CipdInput.Packages = append(toAdd.Properties.CipdInput.Packages, &swarmingpb.CipdPackage{
				PackageName: pkg.PackageName,
				Version:     pkg.Version,
				Path:        pkg.Path,
			})
		}

		for _, dim := range props.Dimensions {
			toAdd.Properties.Dimensions = append(toAdd.Properties.Dimensions, &swarmingpb.StringPair{
				Key:   dim.Key,
				Value: dim.Value,
			})
		}

		ret.TaskSlices = append(ret.TaskSlices, toAdd)
	}

	return ret, nil
}
