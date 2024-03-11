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
	"context"
	"math"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/packedintset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	bqpb "go.chromium.org/luci/swarming/proto/api"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

func seconds(secs int64) *durationpb.Duration {
	return durationpb.New(time.Duration(secs * int64(time.Second)))
}

func sortedDimensionKeys[K ~string, V any](dims map[K]V) []string {
	var keys []string
	for k := range dims {
		keys = append(keys, string(k))
	}
	slices.Sort(keys)
	return keys
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
	dimKeys := sortedDimensionKeys(tp.Dimensions)
	for i, key := range dimKeys {
		values := tp.Dimensions[key]
		sorted := append(make([]string, 0, len(values)), values...)
		slices.Sort(sorted)
		dimensions[i] = &bqpb.StringListPair{
			Key:    key,
			Values: sorted,
		}
	}

	env := make([]*bqpb.StringPair, len(tp.Env))
	envKeys := sortedDimensionKeys(tp.Env)
	for i, key := range envKeys {
		env[i] = &bqpb.StringPair{
			Key:   key,
			Value: tp.Env[key],
		}
	}

	envPaths := make([]*bqpb.StringListPair, len(tp.EnvPrefixes))
	envPathsKeys := sortedDimensionKeys(tp.EnvPrefixes)
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

func taskSlice(ts *model.TaskSlice) *bqpb.TaskSlice {
	// TODO(jonahhooper) implement the property hash calculation
	return &bqpb.TaskSlice{
		Properties:      taskProperties(&ts.Properties),
		Expiration:      seconds(ts.ExpirationSecs),
		WaitForCapacity: ts.WaitForCapacity,
	}
}

func taskRequest(tr *model.TaskRequest) *bqpb.TaskRequest {
	// TODO(jonahhooper) Investigate whether to export root_task_id
	// There is a weird functionality to find root_task_id and root_run_id.
	// It will make our BQ execution quite slow see: https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/5027979/7/swarming/server/model/bqconverter.go#174
	// We should conclusively establish whether we need to do that.
	slices := make([]*bqpb.TaskSlice, len(tr.TaskSlices))
	for i, slice := range tr.TaskSlices {
		slices[i] = taskSlice(&slice)
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

func taskResultCommon(rc *model.TaskResultCommon) *bqpb.TaskResult {
	out := &bqpb.TaskResult{}
	out.ServerVersions = rc.ServerVersions

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
		out.Duration = seconds(int64(math.Round(rc.DurationSecs.Get())))
	}
	if rc.BotIdleSince.IsSet() {
		out.Bot = &bqpb.Bot{
			Info: &bqpb.BotInfo{
				IdleSinceTs: timestamppb.New(rc.BotIdleSince.Get()),
			},
		}
	}

	if len(rc.BotDimensions) != 0 {
		dimKeys := sortedDimensionKeys(rc.BotDimensions)
		botDims := make([]*bqpb.StringListPair, len(rc.BotDimensions))
		if out.Bot == nil {
			out.Bot = &bqpb.Bot{}
		}
		for i, k := range dimKeys {
			v := rc.BotDimensions[k]
			slices.Sort(v)
			switch k {
			case "id":
				out.Bot.BotId = v[0]
			case "pool":
				out.Bot.Pools = append(out.Bot.Pools, v...)
			}
			botDims[i] = &bqpb.StringListPair{
				Key:    k,
				Values: v,
			}
		}
		out.Bot.Dimensions = botDims
	}
	return out
}

func casOverhead(stats *model.CASOperationStats) *bqpb.CASOverhead {
	if stats.DurationSecs == 0 && len(stats.ItemsCold) == 0 && len(stats.ItemsCold) == 0 {
		return nil
	}
	sum := func(ns []int64) int64 {
		t := int64(0)
		for _, n := range ns {
			t += n
		}
		return t
	}
	out := &bqpb.CASOverhead{
		Duration: seconds(int64(stats.DurationSecs)),
	}
	// TODO (crbug.com/329071986): Store precalculated sums in the stats. We only need to compute
	// sum once since ItemsCold and ItemsHot never change once they are set and
	// they genrally can be pretty big, costing more compute power.
	if len(stats.ItemsCold) != 0 {
		ns, _ := packedintset.Unpack(stats.ItemsCold)
		out.Cold = &bqpb.CASEntriesStats{
			NumItems:        int64(len(ns)),
			TotalBytesItems: sum(ns),
		}
	}
	if len(stats.ItemsHot) != 0 {
		ns, _ := packedintset.Unpack(stats.ItemsHot)
		out.Hot = &bqpb.CASEntriesStats{
			NumItems:        int64(len(ns)),
			TotalBytesItems: sum(ns),
		}
	}
	return out
}

func performanceStats(perf *model.PerformanceStats, costUSD float32) *bqpb.TaskPerformance {
	out := &bqpb.TaskPerformance{
		CostUsd: costUSD,
	}
	if perf == nil {
		if costUSD <= 0 {
			return nil
		}
		return out
	}
	if perf.BotOverheadSecs <= 0 {
		return out
	}
	d := func(x float64) time.Duration {
		return time.Duration(float64(time.Second) * x)
	}
	if out.SetupOverhead == nil {
		out.SetupOverhead = &bqpb.TaskSetupOverhead{}
	}
	setupOverhead := time.Duration(0)
	maybeAddToOverhead := func(x float64, overhead *time.Duration) *durationpb.Duration {
		if x <= 0 {
			return nil
		}
		dur := d(x)
		*overhead += dur
		return durationpb.New(dur)
	}
	if s := maybeAddToOverhead(perf.CacheTrim.DurationSecs, &setupOverhead); s != nil {
		out.SetupOverhead.CacheTrim = &bqpb.CacheTrimOverhead{
			Duration: s,
		}
	}
	if s := maybeAddToOverhead(perf.PackageInstallation.DurationSecs, &setupOverhead); s != nil {
		out.SetupOverhead.Cipd = &bqpb.CIPDOverhead{
			Duration: s,
		}
	}
	if s := maybeAddToOverhead(perf.NamedCachesInstall.DurationSecs, &setupOverhead); s != nil {
		out.SetupOverhead.NamedCache = &bqpb.NamedCacheOverhead{
			Duration: s,
		}
	}
	if s := maybeAddToOverhead(perf.IsolatedDownload.DurationSecs, &setupOverhead); s != nil {
		cas := casOverhead(&perf.IsolatedUpload)
		out.SetupOverhead.Cas = cas
		// TODO(https://crbug.com/1510462) remove deprecated Setup field
		out.Setup = &bqpb.TaskOverheadStats{
			Hot:      cas.GetHot(),
			Cold:     cas.GetCold(),
			Duration: cas.GetDuration(),
		}
	}
	if setupOverhead > 0 {
		out.SetupOverhead.Duration = durationpb.New(setupOverhead)
	}
	teardownOverhead := time.Duration(0)
	if out.TeardownOverhead == nil {
		out.TeardownOverhead = &bqpb.TaskTeardownOverhead{}
	}
	if t := maybeAddToOverhead(perf.IsolatedUpload.DurationSecs, &teardownOverhead); t != nil {
		cas := casOverhead(&perf.IsolatedUpload)
		out.TeardownOverhead.Cas = cas
		// TODO(https://crbug.com/1510462) remove deprecated Teardown field
		out.Teardown = &bqpb.TaskOverheadStats{
			Hot:      cas.GetHot(),
			Cold:     cas.GetCold(),
			Duration: cas.GetDuration(),
		}

	}
	if t := maybeAddToOverhead(perf.NamedCachesUninstall.DurationSecs, &teardownOverhead); t != nil {
		out.TeardownOverhead.NamedCache = &bqpb.NamedCacheOverhead{
			Duration: t,
		}
	}
	if t := maybeAddToOverhead(perf.Cleanup.DurationSecs, &teardownOverhead); t != nil {
		out.TeardownOverhead.Cleanup = &bqpb.CleanupOverhead{
			Duration: t,
		}
	}
	if teardownOverhead > 0 {
		out.TeardownOverhead.Duration = durationpb.New(teardownOverhead)
	}
	out.TotalOverhead = durationpb.New(d(perf.BotOverheadSecs))
	out.OtherOverhead = durationpb.New(d(math.Max(perf.BotOverheadSecs-setupOverhead.Seconds()-teardownOverhead.Seconds(), 0.)))
	return out
}

func taskResultSummary(res *model.TaskResultSummary, req *model.TaskRequest, perf *model.PerformanceStats) *bqpb.TaskResult {
	out := taskResultCommon(&res.TaskResultCommon)
	out.CreateTime = timestamppb.New(res.Created)
	out.TaskId = model.RequestKeyToTaskID(res.TaskRequestKey(), model.AsRequest)

	if res.TryNumber.IsSet() {
		out.TryNumber = int32(res.TryNumber.Get())
		out.RunId = model.RequestKeyToTaskID(res.TaskRequestKey(), model.AsRunResult)
		if res.DedupedFrom != "" {
			out.RunId = res.DedupedFrom
		}
	}
	out.ExitCode = int64(res.ExitCode.Get())
	out.DedupedFrom = res.DedupedFrom
	out.Request = taskRequest(req)

	out.State = bqpb.TaskState_TASK_STATE_INVALID
	switch {
	case res.DedupedFrom != "":
		out.State = bqpb.TaskState_DEDUPED
	case res.InternalFailure:
		out.State = bqpb.TaskState_RAN_INTERNAL_FAILURE
	default:
		if state, ok := taskStateMapping[res.State]; ok {
			out.State = state
		}
	}
	// TODO(https://crbug.com/1510462) remove state category as part of this bug
	category := out.State & 0xF0
	out.StateCategory = bqpb.TaskStateCategory(category)

	if res.CIPDPins.Server != "" {
		out.CipdPins = &bqpb.CIPDPins{}
		out.CipdPins.Server = res.CIPDPins.Server
		clPkg := &res.CIPDPins.ClientPackage
		if clPkg.PackageName != "" {
			out.CipdPins.ClientPackage = &bqpb.CIPDPackage{
				PackageName: clPkg.PackageName,
				DestPath:    clPkg.Path,
				Version:     clPkg.Version,
			}
			for _, p := range res.CIPDPins.Packages {
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

	if res.ResultDBInfo.Hostname != "" {
		out.ResultdbInfo = &bqpb.ResultDBInfo{
			Hostname:   res.ResultDBInfo.Hostname,
			Invocation: res.ResultDBInfo.Invocation,
		}
	}

	if res.CASOutputRoot.CASInstance != "" {
		out.CasOutputRoot = &bqpb.CASReference{
			CasInstance: res.CASOutputRoot.CASInstance,
			Digest: &bqpb.Digest{
				Hash:      res.CASOutputRoot.Digest.Hash,
				SizeBytes: res.CASOutputRoot.Digest.SizeBytes,
			},
		}
	}

	out.Performance = performanceStats(perf, float32(res.CostUSD))
	return out
}

func taskResults(ctx context.Context, results []*model.TaskResultSummary) ([]*bqpb.TaskResult, error) {
	eg, ctx := errgroup.WithContext(ctx)

	var stats []*model.PerformanceStats
	statsIdx := map[string]int{}
	k := func(rs *model.TaskResultSummary) string {
		return model.RequestKeyToTaskID(rs.TaskRequestKey(), model.AsRequest)
	}
	for _, rs := range results {
		// TryNumber will only be set under two cases:
		// 1. Task was deduped. Tasks will only have PerformanceStats if they
		// were run to completion. TryNumber = 0 in this case.
		// 2. Task was scheduled to run on a bot. TryNumber = 1 in this case.
		// In case (1), use performance stats from the task which rs was
		// deduped from. So we derive PerformanceStatsKey from `DedupedFrom`
		// field.
		// In case (2),  derive PerformanceStatsKey from request
		// associated with rs.
		// If TryNumber is unset, then there is no way that this bot has
		// performance stats associated with it as it was unscheduled.
		if !rs.TryNumber.IsSet() {
			continue
		}
		reqKey := rs.TaskRequestKey()
		var err error
		if rs.DedupedFrom != "" {
			reqKey, err = model.TaskIDToRequestKey(ctx, rs.DedupedFrom)
			if err != nil {
				return nil, errors.Annotate(err, "could not parse deduped key %s from %s", rs.DedupedFrom, k(rs)).Err()
			}
		}
		// map taskId string to index of the PerformanceStats entity
		// we will be fetching which is associated with that task.
		statsIdx[k(rs)] = len(stats)
		// Use reqKey here because that will refer to dedupedKey of task
		stats = append(stats, &model.PerformanceStats{
			Key: model.PerformanceStatsKey(ctx, reqKey),
		})
	}
	logging.Infof(ctx, "Fetching %d performance stats", len(stats))
	eg.Go(func() error {
		err := datastore.Get(ctx, stats)
		if err == nil {
			return nil
		}

		// Supplying slice stats to datastore.Get will error if any of the
		// supplied performance stats are not found.
		// If that happens, error will be errors.MultiError where the ith
		// error will be the problem getting the ith element.
		var merr errors.MultiError
		if ok := errors.As(err, &merr); ok {
			for i, err := range merr {
				if err == nil {
					continue
				}
				taskID := model.RequestKeyToTaskID(stats[i].TaskRequestKey(), model.AsRunResult)
				if !errors.Is(err, datastore.ErrNoSuchEntity) {
					return errors.Annotate(merr, "failed fetch entity %d with key %s", i, taskID).Err()
				}
				logging.Warningf(ctx, "Expected performance stats to exist for task %s", taskID)
				// setting pointer to nil we ensure that taskResultSummary
				// function will ignore this performance stats object.
				stats[i] = nil
			}
			return nil
		}
		return errors.Annotate(err, "failed to fetch %d perf stats", len(stats)).Err()
	})

	requests := make([]*model.TaskRequest, len(results))
	for i, rs := range results {
		requests[i] = &model.TaskRequest{
			Key: rs.TaskRequestKey(),
		}
	}
	logging.Infof(ctx, "Fetching %d requests", len(requests))
	eg.Go(func() error {
		err := datastore.Get(ctx, requests)
		if err != nil {
			return errors.Annotate(err, "failed to fetch %d requests", len(requests)).Err()
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	bqr := make([]*bqpb.TaskResult, len(results))
	for i, rs := range results {
		var stat *model.PerformanceStats
		if perfIdx, ok := statsIdx[k(rs)]; ok {
			stat = stats[perfIdx]
		}
		bqr[i] = taskResultSummary(rs, requests[i], stat)
	}
	return bqr, nil
}
