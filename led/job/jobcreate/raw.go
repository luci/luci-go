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
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/led/job"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

func casReferenceFromSwarming(cas *swarming.SwarmingRpcsCASReference) *swarmingpb.CASReference {
	if cas == nil {
		return nil
	}
	casRef := &swarmingpb.CASReference{CasInstance: cas.CasInstance}
	if cas.Digest != nil {
		casRef.Digest = &swarmingpb.Digest{
			Hash:      cas.Digest.Hash,
			SizeBytes: cas.Digest.SizeBytes,
		}
	}
	return casRef
}

func cipdPkgsFromSwarming(pkgs *swarming.SwarmingRpcsCipdInput) []*swarmingpb.CIPDPackage {
	if pkgs == nil || len(pkgs.Packages) == 0 {
		return nil
	}
	// NOTE: We thought that ClientPackage would be useful for users,
	// but it turns out that it isn't. Practically, the version of cipd to use for
	// the swarming bot is (and should be) entirely driven by the implementation
	// of the swarming bot.
	ret := make([]*swarmingpb.CIPDPackage, len(pkgs.Packages))
	for i, in := range pkgs.Packages {
		ret[i] = &swarmingpb.CIPDPackage{
			DestPath:    in.Path,
			PackageName: in.PackageName,
			Version:     in.Version,
		}
	}

	return ret
}

func namedCachesFromSwarming(caches []*swarming.SwarmingRpcsCacheEntry) []*swarmingpb.NamedCacheEntry {
	if len(caches) == 0 {
		return nil
	}
	ret := make([]*swarmingpb.NamedCacheEntry, len(caches))
	for i, in := range caches {
		ret[i] = &swarmingpb.NamedCacheEntry{
			Name:     in.Name,
			DestPath: in.Path,
		}
	}
	return ret
}

func dimensionsFromSwarming(dims []*swarming.SwarmingRpcsStringPair) []*swarmingpb.StringListPair {
	if len(dims) == 0 {
		return nil
	}

	intermediate := map[string][]string{}
	for _, dim := range dims {
		intermediate[dim.Key] = append(intermediate[dim.Key], dim.Value)
	}

	ret := make([]*swarmingpb.StringListPair, 0, len(intermediate))
	for key, values := range intermediate {
		ret = append(ret, &swarmingpb.StringListPair{Key: key, Values: values})
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Key < ret[j].Key
	})
	return ret
}

func envFromSwarming(env []*swarming.SwarmingRpcsStringPair) []*swarmingpb.StringPair {
	if len(env) == 0 {
		return nil
	}
	ret := make([]*swarmingpb.StringPair, len(env))
	for i, in := range env {
		ret[i] = &swarmingpb.StringPair{Key: in.Key, Value: in.Value}
	}
	return ret
}

func envPrefixesFromSwarming(envPrefixes []*swarming.SwarmingRpcsStringListPair) []*swarmingpb.StringListPair {
	if len(envPrefixes) == 0 {
		return nil
	}
	ret := make([]*swarmingpb.StringListPair, len(envPrefixes))
	for i, in := range envPrefixes {
		ret[i] = &swarmingpb.StringListPair{Key: in.Key, Values: in.Value}
	}
	return ret
}

func taskPropertiesFromSwarming(ts *swarming.SwarmingRpcsTaskProperties) *swarmingpb.TaskProperties {
	// TODO(iannucci): log that we're dropping SecretBytes?

	return &swarmingpb.TaskProperties{
		CasInputRoot: casReferenceFromSwarming(ts.CasInputRoot),
		CipdInputs:   cipdPkgsFromSwarming(ts.CipdInput),
		NamedCaches:  namedCachesFromSwarming(ts.Caches),
		Command:      ts.Command,
		RelativeCwd:  ts.RelativeCwd,
		// SecretBytes/HasSecretBytes are not provided by the swarming server.
		Dimensions:       dimensionsFromSwarming(ts.Dimensions),
		Env:              envFromSwarming(ts.Env),
		EnvPaths:         envPrefixesFromSwarming(ts.EnvPrefixes),
		Containment:      containmentFromSwarming(ts.Containment),
		ExecutionTimeout: durationpb.New(time.Duration(ts.ExecutionTimeoutSecs) * time.Second),
		IoTimeout:        durationpb.New(time.Duration(ts.IoTimeoutSecs) * time.Second),
		GracePeriod:      durationpb.New(time.Duration(ts.GracePeriodSecs) * time.Second),
		Idempotent:       ts.Idempotent,
		Outputs:          ts.Outputs,
	}
}

func jobDefinitionFromSwarming(sw *job.Swarming, r *swarming.SwarmingRpcsNewTaskRequest) {
	// we ignore r.Properties; TaskSlices are the only thing generated from modern
	// swarming tasks.
	sw.Task = &swarmingpb.TaskRequest{}
	t := sw.Task

	t.TaskSlices = make([]*swarmingpb.TaskSlice, len(r.TaskSlices))
	for i, inslice := range r.TaskSlices {
		outslice := &swarmingpb.TaskSlice{
			Expiration:      durationpb.New(time.Duration(inslice.ExpirationSecs) * time.Second),
			Properties:      taskPropertiesFromSwarming(inslice.Properties),
			WaitForCapacity: inslice.WaitForCapacity,
		}
		t.TaskSlices[i] = outslice
	}

	t.Priority = int32(r.Priority) + 10
	t.ServiceAccount = r.ServiceAccount
	t.Realm = r.Realm
	// CreateTime is unused for new task requests
	// Name is overwritten
	// Tags should not be explicitly provided by the user for the new task
	t.User = r.User
	// TaskId is unpopulated for new task requests
	// ParentTaskId is unpopulated for new task requests
	// ParentRunId is unpopulated for new task requests
	// PubsubNotification is intentionally not propagated.
	t.BotPingTolerance = durationpb.New(time.Duration(r.BotPingToleranceSecs) * time.Second)
}
