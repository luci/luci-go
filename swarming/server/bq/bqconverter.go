// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package bq implements the export of datastore objects to bigquery for analytics
// See go/swarming/bq for more information.
package bq

import (
	// TODO(jonahhooper) use the `slices` module for sorting when it can be used on appengine again.
	"sort"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bqpb "go.chromium.org/luci/swarming/proto/api"
	"go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

func seconds(secs int64) *durationpb.Duration {
	return durationpb.New(time.Duration(secs) * time.Second)
}

func taskPropertiesToBQProto(tp *model.TaskProperties) *bqpb.TaskProperties {
	inputPackages := make([]*bqpb.CIPDPackage, len(tp.CIPDInput.Packages))
	for i, pack := range tp.CIPDInput.Packages {
		inputPackages[i] = &bqpb.CIPDPackage{
			PackageName: pack.PackageName,
			Version:     pack.Version,
			DestPath:    pack.Path,
		}
	}

	namedCaches := make([]*bqpb.NamedCacheEntry, len(tp.Caches))
	for i, cache := range tp.Caches {
		namedCaches[i] = &bqpb.NamedCacheEntry{
			Name:     cache.Name,
			DestPath: cache.Path,
		}
	}

	dimensions := make([]*bqpb.StringListPair, len(tp.Dimensions))
	dimKeys := make([]string, 0, len(tp.Dimensions))
	for key := range tp.Dimensions {
		dimKeys = append(dimKeys, key)
	}
	sort.Strings(dimKeys)
	for i, key := range dimKeys {
		values := tp.Dimensions[key]
		sorted := append(make([]string, 0, len(values)), values...)
		sort.Strings(sorted)
		dimensions[i] = &bqpb.StringListPair{
			Key:    key,
			Values: values,
		}
	}

	env := make([]*bqpb.StringPair, len(tp.Env))
	envKeys := make([]string, 0, len(tp.Env))
	for key := range tp.Env {
		envKeys = append(envKeys, key)
	}
	sort.Strings(envKeys)
	for i, key := range envKeys {
		env[i] = &bqpb.StringPair{
			Key:   key,
			Value: tp.Env[key],
		}
	}

	envPaths := make([]*bqpb.StringListPair, len(tp.EnvPrefixes))
	envPathsKeys := make([]string, 0, len(tp.EnvPrefixes))
	for key := range tp.EnvPrefixes {
		envPathsKeys = append(envPathsKeys, key)
	}
	sort.Strings(envPathsKeys)
	for i, key := range envPathsKeys {
		values := tp.EnvPrefixes[key]
		envPaths[i] = &bqpb.StringListPair{
			Key:    key,
			Values: values,
		}
	}

	containmentType := bqpb.Containment_NOT_SPECIFIED
	switch tp.Containment.ContainmentType {
	case apipb.ContainmentType_NONE:
		containmentType = bqpb.Containment_NONE
	case apipb.ContainmentType_AUTO:
		containmentType = bqpb.Containment_AUTO
	case apipb.ContainmentType_JOB_OBJECT:
		containmentType = bqpb.Containment_JOB_OBJECT
	}

	return &bqpb.TaskProperties{
		CipdInputs:       inputPackages,
		NamedCaches:      namedCaches,
		Command:          tp.Command,
		RelativeCwd:      tp.RelativeCwd,
		HasSecretBytes:   tp.HasSecretBytes,
		Dimensions:       dimensions,
		Env:              env,
		EnvPaths:         envPaths,
		ExecutionTimeout: seconds(tp.ExecutionTimeoutSecs),
		IoTimeout:        seconds(tp.IOTimeoutSecs),
		GracePeriod:      seconds(tp.GracePeriodSecs),
		Idempotent:       tp.Idempotent,
		Outputs:          tp.Outputs,
		CasInputRoot: &bqpb.CASReference{
			CasInstance: tp.CASInputRoot.CASInstance,
			Digest: &bqpb.Digest{
				Hash:      tp.CASInputRoot.Digest.Hash,
				SizeBytes: tp.CASInputRoot.Digest.SizeBytes,
			},
		},
		Containment: &bqpb.Containment{
			ContainmentType: containmentType,
		},
	}
}

func taskSliceToBQ(ts *model.TaskSlice) *bqpb.TaskSlice {
	// TODO(jonahhooper) implement the property hash calculation
	return &bqpb.TaskSlice{
		Properties:      taskPropertiesToBQProto(&ts.Properties),
		Expiration:      seconds(ts.ExpirationSecs),
		WaitForCapacity: ts.WaitForCapacity,
	}
}

func taskRequestToBQ(tr *model.TaskRequest) *bqpb.TaskRequest {
	// TODO(jonahhooper) Investigate whether to export root_task_id
	// There is a weird functionality to find root_task_id and root_run_id.
	// It will make our BQ execution quite slow see: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/5027979/7/swarming/server/model/bqconverter.go#174
	// We should conclusively establish whether we need to do that.
	slices := make([]*bqpb.TaskSlice, len(tr.TaskSlices))
	for i, slice := range tr.TaskSlices {
		slices[i] = taskSliceToBQ(&slice)
	}
	out := bqpb.TaskRequest{
		TaskSlices:     slices,
		Priority:       int32(tr.Priority),
		ServiceAccount: tr.ServiceAccount,
		CreateTime:     timestamppb.New(tr.Created),
		Name:           tr.Name,
		Tags:           tr.Tags,
		User:           tr.User,
		Authenticated:  string(tr.Authenticated),
		Realm:          tr.Realm,
		Resultdb: &bqpb.ResultDBCfg{
			Enable: tr.ResultDB.Enable,
		},
		TaskId: model.RequestKeyToTaskID(tr.Key, model.AsRequest),
		PubsubNotification: &bqpb.PubSub{
			Topic:    tr.PubSubTopic,
			Userdata: tr.PubSubUserData,
		},
		BotPingTolerance: seconds(tr.BotPingToleranceSecs),
	}

	return &out
}
