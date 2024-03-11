// Copyright 2023 The LUCI Authors.
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

package bq

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/packedintset"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	bqpb "go.chromium.org/luci/swarming/proto/api"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func createTaskRequest(key *datastore.Key, testTime time.Time) model.TaskRequest {
	taskSlice := func(val string, exp time.Time) model.TaskSlice {
		return model.TaskSlice{
			Properties: model.TaskProperties{
				Idempotent: true,
				Dimensions: model.TaskDimensions{
					"d1": {"v1", "v2"},
					"d2": {val},
				},
				ExecutionTimeoutSecs: 123,
				GracePeriodSecs:      456,
				IOTimeoutSecs:        789,
				Command:              []string{"run", val},
				RelativeCwd:          "./rel/cwd",
				Env: model.Env{
					"k1": "v1",
					"k2": val,
				},
				EnvPrefixes: model.EnvPrefixes{
					"p1": {"v1", "v2"},
					"p2": {val},
				},
				Caches: []model.CacheEntry{
					{Name: "n1", Path: "p1"},
					{Name: "n2", Path: "p2"},
				},
				CASInputRoot: model.CASReference{
					CASInstance: "cas-inst",
					Digest: model.CASDigest{
						Hash:      "cas-hash",
						SizeBytes: 1234,
					},
				},
				CIPDInput: model.CIPDInput{
					Server: "server",
					ClientPackage: model.CIPDPackage{
						PackageName: "client-package",
						Version:     "client-version",
					},
					Packages: []model.CIPDPackage{
						{
							PackageName: "pkg1",
							Version:     "ver1",
							Path:        "path1",
						},
						{
							PackageName: "pkg2",
							Version:     "ver2",
							Path:        "path2",
						},
					},
				},
				Outputs:        []string{"o1", "o2"},
				HasSecretBytes: true,
				Containment: model.Containment{
					LowerPriority:             true,
					ContainmentType:           apipb.ContainmentType_NOT_SPECIFIED,
					LimitProcesses:            456,
					LimitTotalCommittedMemory: 789,
				},
			},
			ExpirationSecs:  int64(exp.Sub(testTime).Seconds()),
			WaitForCapacity: true,
		}
	}
	return model.TaskRequest{
		Key:     key,
		TxnUUID: "txn-uuid",
		TaskSlices: []model.TaskSlice{
			taskSlice("a", testTime.Add(10*time.Minute)),
			taskSlice("b", testTime.Add(20*time.Minute)),
		},
		Created:              testTime,
		Expiration:           testTime.Add(20 * time.Minute),
		Name:                 "name",
		ParentTaskID:         datastore.Nullable[string, datastore.Indexed]{},
		Authenticated:        "user:authenticated",
		User:                 "user",
		Tags:                 []string{"tag1", "tag2"},
		ManualTags:           []string{"tag1"},
		ServiceAccount:       "service-account",
		Realm:                "realm",
		RealmsEnabled:        true,
		SchedulingAlgorithm:  configpb.Pool_SCHEDULING_ALGORITHM_FIFO,
		Priority:             123,
		BotPingToleranceSecs: 456,
		RBEInstance:          "rbe-instance",
		PubSubTopic:          "pubsub-topic",
		PubSubAuthToken:      "pubsub-auth-token",
		PubSubUserData:       "pubsub-user-data",
		ResultDBUpdateToken:  "resultdb-update-token",
		ResultDB:             model.ResultDBConfig{Enable: true},
		HasBuildTask:         true,
	}
}

func createTaskResultCommon(testTime time.Time) model.TaskResultCommon {
	nullT := datastore.NewIndexedNullable(testTime)
	nullT.Unset()

	return model.TaskResultCommon{
		State:      apipb.TaskState_RUNNING,
		Modified:   testTime,
		BotVersion: "some-version",
		BotDimensions: model.BotDimensions{
			"os":   {"unix", "linux"},
			"cpu":  {"intel", "x86"},
			"pool": {"try:ci", "try:test"},
			"id":   {"bot1"},
		},
		BotIdleSince:        datastore.NewUnindexedOptional(testTime),
		BotLogsCloudProject: "some-cloud-project",
		ServerVersions:      []string{"foo", "bar"},
		CurrentTaskSlice:    1,
		Started:             datastore.NewIndexedNullable(testTime),
		Completed:           datastore.NewIndexedNullable(testTime),
		Abandoned:           nullT,
		DurationSecs:        datastore.NewUnindexedOptional(10.),
		ExitCode:            datastore.NewUnindexedOptional(int64(0)),
		Failure:             false,
		InternalFailure:     false,
		CASOutputRoot: model.CASReference{
			CASInstance: "rbe-cas-instance",
			Digest: model.CASDigest{
				Hash:      "foo-bar-digest",
				SizeBytes: 100,
			},
		},
		ResultDBInfo: model.ResultDBInfo{
			Hostname:   "some-rdb-hostname",
			Invocation: "some-rdb-invocation",
		},
		CIPDPins: model.CIPDInput{
			Server: "some-cipd-server",
			ClientPackage: model.CIPDPackage{
				PackageName: "luci/cipd",
				Path:        "/dev/null",
				Version:     "123",
			},
			Packages: []model.CIPDPackage{
				{
					PackageName: "eat/lunch",
					Path:        "/kitchen/table",
					Version:     "130",
				},
				{
					PackageName: "eat/dinner",
					Path:        "/dinning/room",
					Version:     "630",
				},
			},
		},
	}
}

func createPerformanceStats() *model.PerformanceStats {
	os := func(x float64) model.OperationStats {
		return model.OperationStats{
			DurationSecs: x,
		}
	}
	cos := func(x float64) model.CASOperationStats {
		s := int64(x)
		hot, _ := packedintset.Pack([]int64{s + 1, s + 2})
		cold, _ := packedintset.Pack([]int64{s + 1, s + 2, s + 3})
		return model.CASOperationStats{
			DurationSecs: x,
			ItemsHot:     hot,
			ItemsCold:    cold,
			InitialItems: 10,
			InitialSize:  20,
		}
	}

	return &model.PerformanceStats{
		BotOverheadSecs:      8.,
		CacheTrim:            os(1.),
		PackageInstallation:  os(1.),
		NamedCachesInstall:   os(1.),
		NamedCachesUninstall: os(1.),
		IsolatedDownload:     cos(1.),
		IsolatedUpload:       cos(1.),
		Cleanup:              os(1.),
	}
}

func createBQPerformanceStats() *bqpb.TaskPerformance {
	return &bqpb.TaskPerformance{
		CostUsd:       10.,
		OtherOverhead: seconds(int64(1)),
		TotalOverhead: seconds(int64(8)),
		Setup: &bqpb.TaskOverheadStats{
			Duration: seconds(1),
			Hot: &bqpb.CASEntriesStats{
				NumItems:        2,
				TotalBytesItems: 5,
			},
			Cold: &bqpb.CASEntriesStats{
				NumItems:        3,
				TotalBytesItems: 9,
			},
		},
		Teardown: &bqpb.TaskOverheadStats{
			Duration: seconds(1),
			Hot: &bqpb.CASEntriesStats{
				NumItems:        2,
				TotalBytesItems: 5,
			},
			Cold: &bqpb.CASEntriesStats{
				NumItems:        3,
				TotalBytesItems: 9,
			},
		},
		SetupOverhead: &bqpb.TaskSetupOverhead{
			Duration: seconds(4),
			CacheTrim: &bqpb.CacheTrimOverhead{
				Duration: seconds(1),
			},
			Cipd: &bqpb.CIPDOverhead{
				Duration: seconds(1),
			},
			NamedCache: &bqpb.NamedCacheOverhead{
				Duration: seconds(1),
			},
			Cas: &bqpb.CASOverhead{
				Duration: seconds(1),
				Hot: &bqpb.CASEntriesStats{
					NumItems:        2,
					TotalBytesItems: 5,
				},
				Cold: &bqpb.CASEntriesStats{
					NumItems:        3,
					TotalBytesItems: 9,
				},
			},
		},
		TeardownOverhead: &bqpb.TaskTeardownOverhead{
			Duration: seconds(3),
			Cas: &bqpb.CASOverhead{
				Duration: seconds(1),
				Hot: &bqpb.CASEntriesStats{
					NumItems:        2,
					TotalBytesItems: 5,
				},
				Cold: &bqpb.CASEntriesStats{
					NumItems:        3,
					TotalBytesItems: 9,
				},
			},
			NamedCache: &bqpb.NamedCacheOverhead{
				Duration: seconds(1),
			},
			Cleanup: &bqpb.CleanupOverhead{
				Duration: seconds(1),
			},
		},
	}
}

func createTaskResultSummary(testTime time.Time) *model.TaskResultSummary {
	return &model.TaskResultSummary{
		Created:          testTime,
		TaskResultCommon: createTaskResultCommon(testTime),
		Tags:             []string{"t1", "t2"},
		TryNumber:        datastore.NewIndexedNullable(int64(1)),
		CostUSD:          10.,
		CostSavedUSD:     0,
	}
}

func createBQTaskResultSummary(taskID string, testTime time.Time) *bqpb.TaskResult {
	return &bqpb.TaskResult{
		TaskId:           "65aba3a3e6b99310",
		RunId:            "65aba3a3e6b99311",
		CreateTime:       timestamppb.New(testTime),
		State:            bqpb.TaskState_RUNNING,
		StateCategory:    bqpb.TaskStateCategory_CATEGORY_RUNNING,
		Request:          createBQTaskRequest(taskID, testTime),
		Performance:      createBQPerformanceStats(),
		ServerVersions:   []string{"foo", "bar"},
		CurrentTaskSlice: 0,
		StartTime:        timestamppb.New(testTime),
		EndTime:          timestamppb.New(testTime),
		Duration:         seconds(10.),
		ExitCode:         int64(0),
		TryNumber:        int32(1),
		CipdPins: &bqpb.CIPDPins{
			Server: "some-cipd-server",
			ClientPackage: &bqpb.CIPDPackage{
				PackageName: "luci/cipd",
				DestPath:    "/dev/null",
				Version:     "123",
			},
			Packages: []*bqpb.CIPDPackage{
				{
					PackageName: "eat/lunch",
					DestPath:    "/kitchen/table",
					Version:     "130",
				},
				{
					PackageName: "eat/dinner",
					DestPath:    "/dinning/room",
					Version:     "630",
				},
			},
		},
		CasOutputRoot: &bqpb.CASReference{
			CasInstance: "rbe-cas-instance",
			Digest: &bqpb.Digest{
				Hash:      "foo-bar-digest",
				SizeBytes: 100,
			},
		},
		ResultdbInfo: &bqpb.ResultDBInfo{
			Hostname:   "some-rdb-hostname",
			Invocation: "some-rdb-invocation",
		},
		Bot: &bqpb.Bot{
			Dimensions: []*bqpb.StringListPair{
				{
					Key:    "cpu",
					Values: []string{"intel", "x86"},
				},
				{
					Key:    "id",
					Values: []string{"bot1"},
				},
				{
					Key:    "os",
					Values: []string{"linux", "unix"},
				},
				{
					Key:    "pool",
					Values: []string{"try:ci", "try:test"},
				},
			},
			BotId: "bot1",
			Pools: []string{"try:ci", "try:test"},
			Info: &bqpb.BotInfo{
				IdleSinceTs: timestamppb.New(testTime),
			},
		},
	}
}

func createBQTaskRequest(taskID string, testTime time.Time) *bqpb.TaskRequest {
	taskSlice := func(val string, exp time.Time) *bqpb.TaskSlice {
		return &bqpb.TaskSlice{
			Properties: &bqpb.TaskProperties{
				Idempotent: true,
				Dimensions: []*bqpb.StringListPair{
					{
						Key:    "d1",
						Values: []string{"v1", "v2"},
					},
					{
						Key:    "d2",
						Values: []string{val},
					},
				},
				ExecutionTimeout: seconds(123),
				GracePeriod:      seconds(456),
				IoTimeout:        seconds(789),
				Command:          []string{"run", val},
				RelativeCwd:      "./rel/cwd",
				Env: []*bqpb.StringPair{
					{
						Key:   "k1",
						Value: "v1",
					},
					{
						Key:   "k2",
						Value: val,
					},
				},
				EnvPaths: []*bqpb.StringListPair{
					{
						Key:    "p1",
						Values: []string{"v1", "v2"},
					},
					{
						Key:    "p2",
						Values: []string{val},
					},
				},
				NamedCaches: []*bqpb.NamedCacheEntry{
					{Name: "n1", DestPath: "p1"},
					{Name: "n2", DestPath: "p2"},
				},
				CasInputRoot: &bqpb.CASReference{
					CasInstance: "cas-inst",
					Digest: &bqpb.Digest{
						Hash:      "cas-hash",
						SizeBytes: 1234,
					},
				},
				CipdInputs: []*bqpb.CIPDPackage{
					{
						PackageName: "pkg1",
						Version:     "ver1",
						DestPath:    "path1",
					},
					{
						PackageName: "pkg2",
						Version:     "ver2",
						DestPath:    "path2",
					},
				},
				Outputs:        []string{"o1", "o2"},
				HasSecretBytes: true,
				Containment: &bqpb.Containment{
					ContainmentType: bqpb.Containment_NOT_SPECIFIED,
				},
			},
			Expiration:      seconds(int64(exp.Sub(testTime).Seconds())),
			WaitForCapacity: true,
		}
	}
	return &bqpb.TaskRequest{
		TaskId: taskID,
		TaskSlices: []*bqpb.TaskSlice{
			taskSlice("a", testTime.Add(10*time.Minute)),
			taskSlice("b", testTime.Add(20*time.Minute)),
		},
		Name:             "name",
		CreateTime:       timestamppb.New(testTime),
		ParentTaskId:     "",
		Authenticated:    "user:authenticated",
		User:             "user",
		Tags:             []string{"tag1", "tag2"},
		ServiceAccount:   "service-account",
		Realm:            "realm",
		Priority:         123,
		BotPingTolerance: seconds(456),
		PubsubNotification: &bqpb.PubSub{
			Topic:    "pubsub-topic",
			Userdata: "pubsub-user-data",
		},
		Resultdb: &bqpb.ResultDBCfg{Enable: true},
	}
}

func TestTaskRequestConversion(t *testing.T) {
	t.Parallel()
	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	Convey("Convert TaskRequest with empty parent task", t, func() {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		sampleRequest := createTaskRequest(key, testTime)
		expected := createBQTaskRequest(taskID, testTime)
		actual := taskRequest(&sampleRequest)
		So(actual, ShouldResembleProto, expected)
	})

	Convey("Converting empty EnvPrefixes works", t, func() {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		sampleRequest := createTaskRequest(key, testTime)
		// Set this field to zero
		sampleRequest.TaskSlices[0].Properties.EnvPrefixes = make(model.EnvPrefixes)

		// Set this list to zero too
		expected := createBQTaskRequest(taskID, testTime)
		expected.TaskSlices[0].Properties.EnvPaths = make([]*bqpb.StringListPair, 0)
		actual := taskRequest(&sampleRequest)
		So(err, ShouldBeNil)
		So(actual, ShouldResembleProto, expected)
	})
}

func TestTaskResultConversion(t *testing.T) {
	t.Parallel()
	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	Convey(`Convert completed task with performance stats`, t, func() {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		req := createTaskRequest(key, testTime)
		So(datastore.Put(ctx, &req), ShouldBeNil)
		trs := createTaskResultSummary(testTime)
		trs.Key = model.TaskResultSummaryKey(ctx, key)
		So(datastore.Put(ctx, trs), ShouldBeNil)
		perf := createPerformanceStats()
		perf.Key = model.PerformanceStatsKey(ctx, key)
		So(datastore.Put(ctx, perf), ShouldBeNil)
		expected := []*bqpb.TaskResult{createBQTaskResultSummary(taskID, testTime)}
		actual, err := taskResults(ctx, []*model.TaskResultSummary{trs})
		So(err, ShouldBeNil)
		So(actual, ShouldHaveLength, 1)
		So(actual, ShouldResembleProto, expected)
	})

	Convey(`Do not fail if performance stats "expected" but missing`, t, func() {
		ctx := memory.Use(context.Background())
		var expected []*bqpb.TaskResult

		var km *datastore.Key
		{
			taskID := "55aba3a3e6b99310"
			runID := "55aba3a3e6b99311"
			key, err := model.TaskIDToRequestKey(ctx, taskID)
			So(err, ShouldBeNil)
			req := createTaskRequest(key, testTime)
			So(datastore.Put(ctx, &req), ShouldBeNil)
			trs := createTaskResultSummary(testTime)
			km = model.TaskResultSummaryKey(ctx, key)
			trs.Key = km
			trs.CostUSD = 0
			So(datastore.Put(ctx, trs), ShouldBeNil)
			bpb := createBQTaskResultSummary(taskID, testTime)
			bpb.Performance = nil
			bpb.TaskId = taskID
			bpb.RunId = runID
			expected = append(expected, bpb)
		}

		var kf *datastore.Key
		{
			taskID := "65aba3a3e6b99310"
			key, err := model.TaskIDToRequestKey(ctx, taskID)
			So(err, ShouldBeNil)
			req := createTaskRequest(key, testTime)
			So(datastore.Put(ctx, &req), ShouldBeNil)
			trs := createTaskResultSummary(testTime)
			kf = model.TaskResultSummaryKey(ctx, key)
			trs.Key = kf
			So(datastore.Put(ctx, trs), ShouldBeNil)
			bpb := createBQTaskResultSummary(taskID, testTime)
			perf := createPerformanceStats()
			perf.Key = model.PerformanceStatsKey(ctx, req.Key)
			So(datastore.Put(ctx, perf), ShouldBeNil)
			expected = append(expected, bpb)
		}

		sums := []*model.TaskResultSummary{
			{
				Key: km,
			},
			{
				Key: kf,
			},
		}
		So(datastore.Get(ctx, sums), ShouldBeNil)
		actual, err := taskResults(ctx, sums)
		So(err, ShouldBeNil)
		So(actual, ShouldHaveLength, 2)
		So(actual, ShouldResembleProto, expected)
	})

	Convey(`Convert deduped tasks`, t, func() {
		ctx := memory.Use(context.Background())
		// Create initial task
		ogTaskID := "65aba3a3e6b99310"
		var task *model.TaskResultSummary
		{
			taskID := ogTaskID
			key, err := model.TaskIDToRequestKey(ctx, taskID)
			So(err, ShouldBeNil)
			req := createTaskRequest(key, testTime)
			So(datastore.Put(ctx, &req), ShouldBeNil)
			task = createTaskResultSummary(testTime)
			task.Key = model.TaskResultSummaryKey(ctx, key)
			So(datastore.Put(ctx, task), ShouldBeNil)
			perf := createPerformanceStats()
			perf.Key = model.PerformanceStatsKey(ctx, req.Key)
			So(datastore.Put(ctx, perf), ShouldBeNil)
		}

		// Create deduped task from initial task
		dedupTaskID := "45aba3a3e6c99310"
		var dedupedTask *model.TaskResultSummary
		{
			taskID := dedupTaskID
			key, err := model.TaskIDToRequestKey(ctx, taskID)
			So(err, ShouldBeNil)
			req := createTaskRequest(key, testTime)
			So(datastore.Put(ctx, &req), ShouldBeNil)
			dedupedTask = createTaskResultSummary(testTime)
			dedupedTask.Key = model.TaskResultSummaryKey(ctx, key)
			dedupedTask.DedupedFrom = model.RequestKeyToTaskID(task.TaskRequestKey(), model.AsRunResult)
			dedupedTask.TryNumber = datastore.NewIndexedNullable(int64(0))
			So(datastore.Put(ctx, dedupedTask), ShouldBeNil)
		}

		// Both tasks should share same performance stats
		ogBQ := createBQTaskResultSummary(ogTaskID, testTime)
		dedupedBQ := createBQTaskResultSummary(dedupTaskID, testTime)
		dedupedBQ.TaskId = dedupTaskID
		dedupedBQ.RunId = model.RequestKeyToTaskID(task.TaskRequestKey(), model.AsRunResult)
		dedupedBQ.DedupedFrom = ogBQ.RunId
		dedupedBQ.State = bqpb.TaskState_DEDUPED
		dedupedBQ.StateCategory = bqpb.TaskStateCategory_CATEGORY_NEVER_RAN_DONE
		dedupedBQ.TryNumber = 0
		expected := []*bqpb.TaskResult{
			ogBQ,
			dedupedBQ,
		}
		actual, err := taskResults(ctx, []*model.TaskResultSummary{task, dedupedTask})
		So(err, ShouldBeNil)
		So(actual, ShouldHaveLength, 2)
		So(actual, ShouldResembleProto, expected)
	})

	Convey(`Convert expired task`, t, func() {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		req := createTaskRequest(key, testTime)
		So(datastore.Put(ctx, &req), ShouldBeNil)
		task := &model.TaskResultSummary{
			Key:     model.TaskResultSummaryKey(ctx, key),
			Created: testTime,
			TaskResultCommon: model.TaskResultCommon{
				State:               apipb.TaskState_EXPIRED,
				Modified:            testTime,
				BotVersion:          "some-version",
				BotLogsCloudProject: "some-cloud-project",
				ServerVersions:      []string{"foo", "bar"},
				Completed:           datastore.NewIndexedNullable(testTime),
				Abandoned:           datastore.NewIndexedNullable(testTime),
				Failure:             false,
				InternalFailure:     false,
				ResultDBInfo: model.ResultDBInfo{
					Hostname:   "some-rdb-hostname",
					Invocation: "some-rdb-invocation",
				},
			},
		}
		So(datastore.Put(ctx, task), ShouldBeNil)

		bqTask := &bqpb.TaskResult{
			TaskId:         taskID,
			Request:        createBQTaskRequest(taskID, testTime),
			CreateTime:     timestamppb.New(testTime),
			State:          bqpb.TaskState_EXPIRED,
			StateCategory:  bqpb.TaskStateCategory_CATEGORY_NEVER_RAN_DONE,
			ServerVersions: []string{"foo", "bar"},
			EndTime:        timestamppb.New(testTime),
			AbandonTime:    timestamppb.New(testTime),
			ResultdbInfo: &bqpb.ResultDBInfo{
				Hostname:   "some-rdb-hostname",
				Invocation: "some-rdb-invocation",
			},
		}
		expected := []*bqpb.TaskResult{bqTask}
		actual, err := taskResults(ctx, []*model.TaskResultSummary{task})
		So(err, ShouldBeNil)
		So(actual, ShouldHaveLength, 1)
		So(actual, ShouldResembleProto, expected)
	})

	Convey(`Convert task with internal error`, t, func() {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		runID := "65aba3a3e6b99311"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		So(err, ShouldBeNil)
		req := createTaskRequest(key, testTime)
		So(datastore.Put(ctx, &req), ShouldBeNil)
		task := &model.TaskResultSummary{
			Key:       model.TaskResultSummaryKey(ctx, key),
			Created:   testTime,
			TryNumber: datastore.NewIndexedNullable(int64(1)),
			CostUSD:   1.,
			TaskResultCommon: model.TaskResultCommon{
				State:        apipb.TaskState_KILLED,
				BotIdleSince: datastore.NewUnindexedOptional(testTime),
				BotDimensions: model.BotDimensions{
					"os":   {"unix", "linux"},
					"cpu":  {"intel", "x86"},
					"pool": {"try:ci", "try:test"},
					"id":   {"bot1"},
				},
				ExitCode:            datastore.NewUnindexedOptional(int64(-1)),
				Modified:            testTime,
				BotVersion:          "some-version",
				BotLogsCloudProject: "some-cloud-project",
				ServerVersions:      []string{"foo", "bar"},
				Completed:           datastore.NewIndexedNullable(testTime),
				Abandoned:           datastore.NewIndexedNullable(testTime),
				Failure:             true,
				InternalFailure:     true,
				ResultDBInfo: model.ResultDBInfo{
					Hostname:   "some-rdb-hostname",
					Invocation: "some-rdb-invocation",
				},
			},
		}
		So(datastore.Put(ctx, task), ShouldBeNil)

		bqTask := &bqpb.TaskResult{
			TaskId:         taskID,
			RunId:          runID,
			Request:        createBQTaskRequest(taskID, testTime),
			TryNumber:      1,
			ExitCode:       -1,
			CreateTime:     timestamppb.New(testTime),
			State:          bqpb.TaskState_RAN_INTERNAL_FAILURE,
			StateCategory:  bqpb.TaskStateCategory_CATEGORY_TRANSIENT_DONE,
			ServerVersions: []string{"foo", "bar"},
			EndTime:        timestamppb.New(testTime),
			AbandonTime:    timestamppb.New(testTime),
			Performance: &bqpb.TaskPerformance{
				CostUsd: 1.,
			},
			Bot: &bqpb.Bot{
				Dimensions: []*bqpb.StringListPair{
					{
						Key:    "cpu",
						Values: []string{"intel", "x86"},
					},
					{
						Key:    "id",
						Values: []string{"bot1"},
					},
					{
						Key:    "os",
						Values: []string{"linux", "unix"},
					},
					{
						Key:    "pool",
						Values: []string{"try:ci", "try:test"},
					},
				},
				BotId: "bot1",
				Pools: []string{"try:ci", "try:test"},
				Info: &bqpb.BotInfo{
					IdleSinceTs: timestamppb.New(testTime),
				},
			},
			ResultdbInfo: &bqpb.ResultDBInfo{
				Hostname:   "some-rdb-hostname",
				Invocation: "some-rdb-invocation",
			},
		}
		expected := []*bqpb.TaskResult{bqTask}
		actual, err := taskResults(ctx, []*model.TaskResultSummary{task})
		So(err, ShouldBeNil)
		So(actual, ShouldHaveLength, 1)
		So(actual, ShouldResembleProto, expected)
	})
}
