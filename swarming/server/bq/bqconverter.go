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

// Package bq implements the export of datastore objects to bigquery.
//
// See go/swarming/bq for more information.
package bq

import (
	"encoding/hex"
	"math"
	"slices"

	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	bqpb "go.chromium.org/luci/swarming/proto/bq"
	"go.chromium.org/luci/swarming/server/model"
)

func sortedKeys[K ~string, V any](m map[K]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, string(k))
	}
	slices.Sort(keys)
	return keys
}

func runIDToTaskID(runID string) string {
	if runID == "" {
		return ""
	}
	return runID[:len(runID)-1] + "0"
}

func taskProperties(tp *model.TaskProperties) *bqpb.TaskProperties {
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
	for i, key := range sortedKeys(tp.Dimensions) {
		values := tp.Dimensions[key]
		sorted := append(make([]string, 0, len(values)), values...)
		slices.Sort(sorted)
		dimensions[i] = &bqpb.StringListPair{
			Key:    key,
			Values: sorted,
		}
	}

	env := make([]*bqpb.StringPair, len(tp.Env))
	for i, key := range sortedKeys(tp.Env) {
		env[i] = &bqpb.StringPair{
			Key:   key,
			Value: tp.Env[key],
		}
	}

	envPaths := make([]*bqpb.StringListPair, len(tp.EnvPrefixes))
	for i, key := range sortedKeys(tp.EnvPrefixes) {
		values := tp.EnvPrefixes[key]
		envPaths[i] = &bqpb.StringListPair{
			Key:    key,
			Values: values,
		}
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
		ExecutionTimeout: float64(tp.ExecutionTimeoutSecs),
		IoTimeout:        float64(tp.IOTimeoutSecs),
		GracePeriod:      float64(tp.GracePeriodSecs),
		Idempotent:       tp.Idempotent,
		Outputs:          tp.Outputs,
		CasInputRoot: &apipb.CASReference{
			CasInstance: tp.CASInputRoot.CASInstance,
			Digest: &apipb.Digest{
				Hash:      tp.CASInputRoot.Digest.Hash,
				SizeBytes: tp.CASInputRoot.Digest.SizeBytes,
			},
		},
		Containment: &apipb.Containment{
			ContainmentType:           tp.Containment.ContainmentType,
			LowerPriority:             tp.Containment.LowerPriority,
			LimitProcesses:            tp.Containment.LimitProcesses,
			LimitTotalCommittedMemory: tp.Containment.LimitTotalCommittedMemory,
		},
	}
}

func taskSlice(ts *model.TaskSlice) *bqpb.TaskSlice {
	return &bqpb.TaskSlice{
		Properties:      taskProperties(&ts.Properties),
		Expiration:      float64(ts.ExpirationSecs),
		WaitForCapacity: ts.WaitForCapacity,
		PropertiesHash:  hex.EncodeToString(ts.PropertiesHash),
	}
}

func taskRequest(tr *model.TaskRequest) *bqpb.TaskRequest {
	slices := make([]*bqpb.TaskSlice, len(tr.TaskSlices))
	for i, slice := range tr.TaskSlices {
		slices[i] = taskSlice(&slice)
	}
	return &bqpb.TaskRequest{
		TaskSlices:     slices,
		Priority:       int32(tr.Priority),
		ServiceAccount: tr.ServiceAccount,
		CreateTime:     timestamppb.New(tr.Created),
		Name:           tr.Name,
		Tags:           tr.Tags,
		User:           tr.User,
		Authenticated:  string(tr.Authenticated),
		Realm:          tr.Realm,
		Resultdb:       &apipb.ResultDBCfg{Enable: tr.ResultDB.Enable},
		TaskId:         model.RequestKeyToTaskID(tr.Key, model.AsRequest),
		ParentTaskId:   runIDToTaskID(tr.ParentTaskID.Get()),
		ParentRunId:    tr.ParentTaskID.Get(),
		RootTaskId:     runIDToTaskID(tr.RootTaskID),
		RootRunId:      tr.RootTaskID,
		PubsubNotification: &bqpb.PubSub{
			Topic:    tr.PubSubTopic,
			Userdata: tr.PubSubUserData,
		},
		BotPingTolerance: float64(tr.BotPingToleranceSecs),
	}
}

var taskStateMapping = map[apipb.TaskState]bqpb.TaskState{
	apipb.TaskState_PENDING:      bqpb.TaskState_PENDING,
	apipb.TaskState_RUNNING:      bqpb.TaskState_RUNNING,
	apipb.TaskState_COMPLETED:    bqpb.TaskState_COMPLETED,
	apipb.TaskState_TIMED_OUT:    bqpb.TaskState_TIMED_OUT,
	apipb.TaskState_KILLED:       bqpb.TaskState_KILLED,
	apipb.TaskState_EXPIRED:      bqpb.TaskState_EXPIRED,
	apipb.TaskState_CANCELED:     bqpb.TaskState_CANCELED,
	apipb.TaskState_NO_RESOURCE:  bqpb.TaskState_NO_RESOURCE,
	apipb.TaskState_CLIENT_ERROR: bqpb.TaskState_CLIENT_ERROR,
}

func taskResultCommon(rc *model.TaskResultCommon, req *model.TaskRequest) *bqpb.TaskResult {
	out := &bqpb.TaskResult{
		TaskId:         model.RequestKeyToTaskID(req.Key, model.AsRequest),
		CreateTime:     timestamppb.New(req.Created),
		Request:        taskRequest(req),
		ServerVersions: rc.ServerVersions,
		ExitCode:       int64(rc.ExitCode.Get()),
	}

	if rc.Started.IsSet() {
		out.StartTime = timestamppb.New(rc.Started.Get())
	}
	if rc.Completed.IsSet() {
		out.EndTime = timestamppb.New(rc.Completed.Get())
	}
	if rc.Abandoned.IsSet() {
		out.AbandonTime = timestamppb.New(rc.Abandoned.Get())
	}
	if rc.DurationSecs.IsSet() {
		out.Duration = rc.DurationSecs.Get()
	}
	if rc.BotIdleSince.IsSet() {
		out.Bot = &bqpb.Bot{
			Info: &bqpb.BotInfo{
				IdleSinceTs: timestamppb.New(rc.BotIdleSince.Get()),
			},
		}
	}

	if len(rc.BotDimensions) != 0 {
		if out.Bot == nil {
			out.Bot = &bqpb.Bot{}
		}
		out.Bot.Dimensions = make([]*bqpb.StringListPair, len(rc.BotDimensions))
		for i, k := range sortedKeys(rc.BotDimensions) {
			v := rc.BotDimensions[k]
			slices.Sort(v)
			switch k {
			case "id":
				out.Bot.BotId = v[0]
			case "pool":
				out.Bot.Pools = append(out.Bot.Pools, v...)
			}
			out.Bot.Dimensions[i] = &bqpb.StringListPair{
				Key:    k,
				Values: v,
			}
		}
	}

	out.State = bqpb.TaskState_TASK_STATE_INVALID
	if rc.InternalFailure {
		out.State = bqpb.TaskState_RAN_INTERNAL_FAILURE
	} else if state, ok := taskStateMapping[rc.State]; ok {
		out.State = state
	}
	// TODO(https://crbug.com/1510462) remove state category as part of this bug
	out.StateCategory = bqpb.TaskStateCategory(out.State & 0xF0)

	if rc.CIPDPins.Server != "" {
		out.CipdPins = &bqpb.CIPDPins{Server: rc.CIPDPins.Server}
		if rc.CIPDPins.ClientPackage.PackageName != "" {
			out.CipdPins.ClientPackage = &bqpb.CIPDPackage{
				PackageName: rc.CIPDPins.ClientPackage.PackageName,
				DestPath:    rc.CIPDPins.ClientPackage.Path,
				Version:     rc.CIPDPins.ClientPackage.Version,
			}
			for _, p := range rc.CIPDPins.Packages {
				if p.PackageName == "" {
					continue
				}
				out.CipdPins.Packages = append(out.CipdPins.Packages, &bqpb.CIPDPackage{
					PackageName: p.PackageName,
					DestPath:    p.Path,
					Version:     p.Version,
				})
			}
		}
	}

	if rc.ResultDBInfo.Hostname != "" {
		out.ResultdbInfo = &apipb.ResultDBInfo{
			Hostname:   rc.ResultDBInfo.Hostname,
			Invocation: rc.ResultDBInfo.Invocation,
		}
	}

	if rc.CASOutputRoot.CASInstance != "" {
		out.CasOutputRoot = &apipb.CASReference{
			CasInstance: rc.CASOutputRoot.CASInstance,
			Digest: &apipb.Digest{
				Hash:      rc.CASOutputRoot.Digest.Hash,
				SizeBytes: rc.CASOutputRoot.Digest.SizeBytes,
			},
		}
	}

	return out
}

func casOverhead(stats *model.CASOperationStats) *bqpb.CASOverhead {
	pb, _ := stats.ToProto()
	if pb == nil {
		return nil // either empty or broken, just skip
	}
	return &bqpb.CASOverhead{
		Duration: float64(pb.Duration),
		Cold: &bqpb.CASEntriesStats{
			NumItems:        pb.NumItemsCold,
			TotalBytesItems: pb.TotalBytesItemsCold,
			Items:           nil, // intentionally not exported to BQ
		},
		Hot: &bqpb.CASEntriesStats{
			NumItems:        pb.NumItemsHot,
			TotalBytesItems: pb.TotalBytesItemsHot,
			Items:           nil, // intentionally not exported to BQ
		},
	}
}

func performanceStats(perf *model.PerformanceStats, costUSD float32) *bqpb.TaskPerformance {
	out := &bqpb.TaskPerformance{CostUsd: costUSD}
	if perf == nil || perf.BotOverheadSecs == 0 {
		if costUSD == 0 {
			return nil
		}
		return out
	}

	maybeAddToOverhead := func(x float64, overhead *float64) float64 {
		if x <= 0.0 {
			return 0.0
		}
		*overhead += x
		return x
	}

	var setupOverhead float64
	var teardownOverhead float64

	out.SetupOverhead = &bqpb.TaskSetupOverhead{}
	out.TeardownOverhead = &bqpb.TaskTeardownOverhead{}

	// Add up all setup overhead.
	if s := maybeAddToOverhead(perf.CacheTrim.DurationSecs, &setupOverhead); s != 0.0 {
		out.SetupOverhead.CacheTrim = &bqpb.CacheTrimOverhead{Duration: s}
	}
	if s := maybeAddToOverhead(perf.PackageInstallation.DurationSecs, &setupOverhead); s != 0.0 {
		out.SetupOverhead.Cipd = &bqpb.CIPDOverhead{Duration: s}
	}
	if s := maybeAddToOverhead(perf.NamedCachesInstall.DurationSecs, &setupOverhead); s != 0.0 {
		out.SetupOverhead.NamedCache = &bqpb.NamedCacheOverhead{Duration: s}
	}
	if s := maybeAddToOverhead(perf.IsolatedDownload.DurationSecs, &setupOverhead); s != 0.0 {
		cas := casOverhead(&perf.IsolatedDownload)
		out.SetupOverhead.Cas = cas
		// TODO(https://crbug.com/1510462) remove deprecated Setup field
		out.Setup = &bqpb.TaskOverheadStats{
			Hot:      cas.GetHot(),
			Cold:     cas.GetCold(),
			Duration: cas.GetDuration(),
		}
	}

	// Add up all teardown overhead.
	if t := maybeAddToOverhead(perf.IsolatedUpload.DurationSecs, &teardownOverhead); t != 0.0 {
		cas := casOverhead(&perf.IsolatedUpload)
		out.TeardownOverhead.Cas = cas
		// TODO(https://crbug.com/1510462) remove deprecated Teardown field
		out.Teardown = &bqpb.TaskOverheadStats{
			Hot:      cas.GetHot(),
			Cold:     cas.GetCold(),
			Duration: cas.GetDuration(),
		}
	}
	if t := maybeAddToOverhead(perf.NamedCachesUninstall.DurationSecs, &teardownOverhead); t != 0.0 {
		out.TeardownOverhead.NamedCache = &bqpb.NamedCacheOverhead{Duration: t}
	}
	if t := maybeAddToOverhead(perf.Cleanup.DurationSecs, &teardownOverhead); t != 0.0 {
		out.TeardownOverhead.Cleanup = &bqpb.CleanupOverhead{Duration: t}
	}

	out.SetupOverhead.Duration = setupOverhead
	out.TeardownOverhead.Duration = teardownOverhead
	out.TotalOverhead = perf.BotOverheadSecs
	out.OtherOverhead = math.Max(perf.BotOverheadSecs-setupOverhead-teardownOverhead, 0.0)

	return out
}

func taskResultSummary(res *model.TaskResultSummary, req *model.TaskRequest, perf *model.PerformanceStats) *bqpb.TaskResult {
	out := taskResultCommon(&res.TaskResultCommon, req)
	out.Performance = performanceStats(perf, float32(res.CostUSD))
	out.TryNumber = int32(res.TryNumber.Get())
	if out.TryNumber != 0 {
		out.RunId = model.RequestKeyToTaskID(req.Key, model.AsRunResult)
	}
	if res.DedupedFrom != "" {
		out.DedupedFrom = res.DedupedFrom
		out.RunId = res.DedupedFrom
		out.State = bqpb.TaskState_DEDUPED
		out.StateCategory = bqpb.TaskStateCategory_CATEGORY_NEVER_RAN_DONE
	}
	return out
}

func taskRunResult(res *model.TaskRunResult, req *model.TaskRequest, perf *model.PerformanceStats) *bqpb.TaskResult {
	out := taskResultCommon(&res.TaskResultCommon, req)
	out.Performance = performanceStats(perf, float32(res.CostUSD))
	out.TryNumber = 1
	out.RunId = model.RequestKeyToTaskID(req.Key, model.AsRunResult)
	return out
}
