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

	bqtp := &bqpb.TaskProperties{
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
	}

	if tp.CASInputRoot.Digest.Hash != "" {
		bqtp.CasInputRoot = &apipb.CASReference{
			CasInstance: tp.CASInputRoot.CASInstance,
			Digest: &apipb.Digest{
				Hash:      tp.CASInputRoot.Digest.Hash,
				SizeBytes: tp.CASInputRoot.Digest.SizeBytes,
			},
		}
	}

	if tp.Containment.ContainmentType != apipb.Containment_NOT_SPECIFIED && tp.Containment.ContainmentType != apipb.Containment_NONE {
		bqtp.Containment = &apipb.Containment{
			ContainmentType:           tp.Containment.ContainmentType,
			LowerPriority:             tp.Containment.LowerPriority,
			LimitProcesses:            tp.Containment.LimitProcesses,
			LimitTotalCommittedMemory: tp.Containment.LimitTotalCommittedMemory,
		}
	}

	return bqtp
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
	bqtr := &bqpb.TaskRequest{
		TaskSlices:       slices,
		Priority:         int32(tr.Priority),
		ServiceAccount:   tr.ServiceAccount,
		CreateTime:       timestamppb.New(tr.Created),
		Name:             tr.Name,
		Tags:             tr.Tags,
		User:             tr.User,
		Authenticated:    string(tr.Authenticated),
		Realm:            tr.Realm,
		Resultdb:         &apipb.ResultDBCfg{Enable: tr.ResultDB.Enable},
		TaskId:           model.RequestKeyToTaskID(tr.Key, model.AsRequest),
		ParentTaskId:     runIDToTaskID(tr.ParentTaskID.Get()),
		ParentRunId:      tr.ParentTaskID.Get(),
		RootTaskId:       runIDToTaskID(tr.RootTaskID),
		RootRunId:        tr.RootTaskID,
		BotPingTolerance: float64(tr.BotPingToleranceSecs),
	}
	if tr.PubSubTopic != "" {
		bqtr.PubsubNotification = &bqpb.PubSub{
			Topic:    tr.PubSubTopic,
			Userdata: tr.PubSubUserData,
		}
	}
	return bqtr
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
		TaskId:           model.RequestKeyToTaskID(req.Key, model.AsRequest),
		CreateTime:       timestamppb.New(req.Created),
		CurrentTaskSlice: int32(rc.CurrentTaskSlice),
		Request:          taskRequest(req),
		ServerVersions:   rc.ServerVersions,
		ExitCode:         int64(rc.ExitCode.Get()),
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

	cipdServer := rc.CIPDPins.Server
	if cipdServer == "" && len(req.TaskSlices) != 0 {
		cipdServer = req.TaskSlices[0].Properties.CIPDInput.Server
	}
	if cipdServer != "" || rc.CIPDPins.IsPopulated() {
		out.CipdPins = &bqpb.CIPDPins{Server: cipdServer}
		if rc.CIPDPins.ClientPackage.PackageName != "" {
			out.CipdPins.ClientPackage = &bqpb.CIPDPackage{
				PackageName: rc.CIPDPins.ClientPackage.PackageName,
				DestPath:    rc.CIPDPins.ClientPackage.Path,
				Version:     rc.CIPDPins.ClientPackage.Version,
			}
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

	if rc.ResultDBInfo.Hostname != "" {
		out.ResultdbInfo = &apipb.ResultDBInfo{
			Hostname:   rc.ResultDBInfo.Hostname,
			Invocation: rc.ResultDBInfo.Invocation,
		}
	}

	if rc.CASOutputRoot.Digest.Hash != "" {
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

	// Deprecated, but still potentially used fields.
	// TODO(https://crbug.com/1510462): Remove them at some point.
	out.Setup = &bqpb.TaskOverheadStats{}
	out.Teardown = &bqpb.TaskOverheadStats{}

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
		out.Setup.Hot = cas.GetHot()
		out.Setup.Cold = cas.GetCold()
	}

	// Add up all teardown overhead.
	if t := maybeAddToOverhead(perf.IsolatedUpload.DurationSecs, &teardownOverhead); t != 0.0 {
		cas := casOverhead(&perf.IsolatedUpload)
		out.TeardownOverhead.Cas = cas
		out.Teardown.Hot = cas.GetHot()
		out.Teardown.Cold = cas.GetCold()
	}
	if t := maybeAddToOverhead(perf.NamedCachesUninstall.DurationSecs, &teardownOverhead); t != 0.0 {
		out.TeardownOverhead.NamedCache = &bqpb.NamedCacheOverhead{Duration: t}
	}
	if t := maybeAddToOverhead(perf.Cleanup.DurationSecs, &teardownOverhead); t != 0.0 {
		out.TeardownOverhead.Cleanup = &bqpb.CleanupOverhead{Duration: t}
	}

	out.SetupOverhead.Duration = setupOverhead
	out.TeardownOverhead.Duration = teardownOverhead
	out.Setup.Duration = setupOverhead
	out.Teardown.Duration = teardownOverhead
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

// Datastore event type => BQ exports event type.
var botEventMapping = map[model.BotEventType]bqpb.BotEventType{
	model.BotEventConnected: bqpb.BotEventType_BOT_NEW_SESSION,
	model.BotEventError:     bqpb.BotEventType_BOT_HOOK_ERROR,
	model.BotEventLog:       bqpb.BotEventType_BOT_HOOK_LOG,

	model.BotEventMissing:   bqpb.BotEventType_BOT_MISSING,
	model.BotEventRebooting: bqpb.BotEventType_BOT_REBOOTING_HOST,
	model.BotEventShutdown:  bqpb.BotEventType_BOT_SHUTDOWN,
	model.BotEventTerminate: bqpb.BotEventType_INSTRUCT_TERMINATE_BOT,
	model.BotEventRestart:   bqpb.BotEventType_INSTRUCT_RESTART_BOT,

	model.BotEventSleep:         bqpb.BotEventType_INSTRUCT_IDLE,
	model.BotEventTask:          bqpb.BotEventType_INSTRUCT_START_TASK,
	model.BotEventUpdate:        bqpb.BotEventType_INSTRUCT_UPDATE_BOT_CODE,
	model.BotEventTaskCompleted: bqpb.BotEventType_TASK_COMPLETED,
	model.BotEventTaskError:     bqpb.BotEventType_TASK_INTERNAL_FAILURE,
	model.BotEventTaskKilled:    bqpb.BotEventType_TASK_KILLED,

	// These values are not registered in the BQ API.
	model.BotEventIdle:       bqpb.BotEventType_BOT_EVENT_TYPE_UNSPECIFIED,
	model.BotEventPolling:    bqpb.BotEventType_BOT_EVENT_TYPE_UNSPECIFIED,
	model.BotEventTaskUpdate: bqpb.BotEventType_BOT_EVENT_TYPE_UNSPECIFIED,
}

func botEvent(ev *model.BotEvent) *bqpb.BotEvent {
	dims := model.DimensionsFlatToPb(ev.Dimensions)

	bot := &bqpb.Bot{
		BotId:         ev.Key.Parent().StringID(),
		SessionId:     ev.SessionID,
		CurrentTaskId: ev.TaskID,
		Dimensions:    make([]*bqpb.StringListPair, len(dims)),
		Info: &bqpb.BotInfo{
			Supplemental:    string(ev.State.JSON),
			Version:         ev.Version,
			ExternalIp:      ev.ExternalIP,
			AuthenticatedAs: string(ev.AuthenticatedAs),
		},
	}

	for i, dim := range dims {
		if dim.Key == "pool" {
			bot.Pools = dim.Value
		}
		bot.Dimensions[i] = &bqpb.StringListPair{
			Key:    dim.Key,
			Values: dim.Value,
		}
	}

	if ev.IdleSince.IsSet() {
		bot.Info.IdleSinceTs = timestamppb.New(ev.IdleSince.Get())
	}
	if ev.LastSeen.IsSet() && ev.EventType == model.BotEventMissing {
		bot.Info.LastSeenTs = timestamppb.New(ev.LastSeen.Get())
	}

	if ev.Quarantined {
		bot.Status = bqpb.BotStatusType_QUARANTINED_BY_BOT
		bot.StatusMsg = ev.QuarantineMessage()
	} else if ev.Maintenance != "" {
		bot.Status = bqpb.BotStatusType_OVERHEAD_MAINTENANCE_EXTERNAL
		bot.StatusMsg = ev.Maintenance
	} else if ev.EventType == model.BotEventMissing {
		bot.Status = bqpb.BotStatusType_MISSING
	} else if ev.IsIdle() {
		bot.Status = bqpb.BotStatusType_IDLE
	} else if ev.TaskID != "" {
		bot.Status = bqpb.BotStatusType_BUSY
	}

	return &bqpb.BotEvent{
		EventTime: timestamppb.New(ev.Timestamp),
		Bot:       bot,
		Event:     botEventMapping[ev.EventType],
		EventMsg:  ev.Message,
	}
}
