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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	bqpb "go.chromium.org/luci/swarming/proto/bq"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/botstate"
	"go.chromium.org/luci/swarming/server/model"
)

func createTaskRequest(key *datastore.Key, testTime time.Time) *model.TaskRequest {
	taskSlice := func(val string, exp time.Duration) model.TaskSlice {
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
					ContainmentType:           apipb.Containment_JOB_OBJECT,
					LimitProcesses:            456,
					LimitTotalCommittedMemory: 789,
				},
			},
			ExpirationSecs:  int64(exp.Seconds()),
			WaitForCapacity: true,
			PropertiesHash:  []byte{1, 2, 3, 4, 5},
		}
	}
	return &model.TaskRequest{
		Key:     key,
		TxnUUID: "txn-uuid",
		TaskSlices: []model.TaskSlice{
			taskSlice("a", 10*time.Minute),
			taskSlice("b", 20*time.Minute),
		},
		Created:              testTime,
		Expiration:           testTime.Add(20 * time.Minute),
		Name:                 "name",
		ParentTaskID:         datastore.NewIndexedNullable("aaaaaaaaf1"),
		RootTaskID:           "bbbbbbbbf1",
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

func createTaskResultCommon(testTime time.Time) *model.TaskResultCommon {
	return &model.TaskResultCommon{
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
		DurationSecs:        datastore.NewUnindexedOptional(10.456),
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
	cos := func(dur float64, cold, hot []int64) model.CASOperationStats {
		hotPacked, _ := packedintset.Pack(hot)
		coldPacked, _ := packedintset.Pack(cold)
		return model.CASOperationStats{
			DurationSecs: dur,
			ItemsHot:     hotPacked,
			ItemsCold:    coldPacked,
			InitialItems: 10,
			InitialSize:  20,
		}
	}

	return &model.PerformanceStats{
		BotOverheadSecs:      123.,
		CacheTrim:            os(1.5),
		PackageInstallation:  os(2.5),
		NamedCachesInstall:   os(3.5),
		NamedCachesUninstall: os(4.5),
		IsolatedDownload:     cos(5.5, []int64{1, 2}, []int64{4, 5, 6}),
		IsolatedUpload:       cos(6.5, []int64{7, 8}, []int64{9, 10, 11}),
		Cleanup:              os(7.5),
	}
}

func createBQPerformanceStats(costUSD float32) *bqpb.TaskPerformance {
	casStats := func(items []int64) *bqpb.CASEntriesStats {
		sum := int64(0)
		for _, i := range items {
			sum += i
		}
		return &bqpb.CASEntriesStats{
			NumItems:        int64(len(items)),
			TotalBytesItems: sum,
			Items:           nil, // not actually exported
		}
	}

	casSetup := &bqpb.CASOverhead{
		Duration: 5.5,
		Cold:     casStats([]int64{1, 2}),
		Hot:      casStats([]int64{4, 5, 6}),
	}

	casTeardown := &bqpb.CASOverhead{
		Duration: 6.5,
		Cold:     casStats([]int64{7, 8}),
		Hot:      casStats([]int64{9, 10, 11}),
	}

	return &bqpb.TaskPerformance{
		CostUsd:       costUSD,
		OtherOverhead: 91.5,
		TotalOverhead: 123.,
		Setup: &bqpb.TaskOverheadStats{
			Duration: 13.0, // total
			Hot:      casSetup.Hot,
			Cold:     casSetup.Cold,
		},
		Teardown: &bqpb.TaskOverheadStats{
			Duration: 18.5, // total
			Hot:      casTeardown.Hot,
			Cold:     casTeardown.Cold,
		},
		SetupOverhead: &bqpb.TaskSetupOverhead{
			Duration: 13.0,
			CacheTrim: &bqpb.CacheTrimOverhead{
				Duration: 1.5,
			},
			Cipd: &bqpb.CIPDOverhead{
				Duration: 2.5,
			},
			NamedCache: &bqpb.NamedCacheOverhead{
				Duration: 3.5,
			},
			Cas: casSetup,
		},
		TeardownOverhead: &bqpb.TaskTeardownOverhead{
			Duration: 18.5,
			Cas:      casTeardown,
			NamedCache: &bqpb.NamedCacheOverhead{
				Duration: 4.5,
			},
			Cleanup: &bqpb.CleanupOverhead{
				Duration: 7.5,
			},
		},
	}
}

// createBQTaskResultBase creates *bqpb.TaskResult matching TaskResultCommon.
//
// Following fields are not populated: RunId, TryNumber, Performance.
func createBQTaskResultBase(taskID string, testTime time.Time) *bqpb.TaskResult {
	return &bqpb.TaskResult{
		TaskId:           taskID,
		CreateTime:       timestamppb.New(testTime),
		State:            bqpb.TaskState_RUNNING,
		StateCategory:    bqpb.TaskStateCategory_CATEGORY_RUNNING,
		Request:          createBQTaskRequest(taskID, testTime),
		ServerVersions:   []string{"foo", "bar"},
		CurrentTaskSlice: 1,
		StartTime:        timestamppb.New(testTime),
		EndTime:          timestamppb.New(testTime),
		Duration:         10.456,
		ExitCode:         int64(0),
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
		CasOutputRoot: &apipb.CASReference{
			CasInstance: "rbe-cas-instance",
			Digest: &apipb.Digest{
				Hash:      "foo-bar-digest",
				SizeBytes: 100,
			},
		},
		ResultdbInfo: &apipb.ResultDBInfo{
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
	taskSlice := func(val string, exp time.Duration) *bqpb.TaskSlice {
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
				ExecutionTimeout: 123.0,
				GracePeriod:      456.0,
				IoTimeout:        789.0,
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
				CasInputRoot: &apipb.CASReference{
					CasInstance: "cas-inst",
					Digest: &apipb.Digest{
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
				Containment: &apipb.Containment{
					ContainmentType:           apipb.Containment_JOB_OBJECT,
					LowerPriority:             true,
					LimitProcesses:            456,
					LimitTotalCommittedMemory: 789,
				},
			},
			Expiration:      exp.Seconds(),
			WaitForCapacity: true,
			PropertiesHash:  "0102030405",
		}
	}
	return &bqpb.TaskRequest{
		TaskId:       taskID,
		ParentRunId:  "aaaaaaaaf1",
		ParentTaskId: "aaaaaaaaf0",
		RootRunId:    "bbbbbbbbf1",
		RootTaskId:   "bbbbbbbbf0",
		TaskSlices: []*bqpb.TaskSlice{
			taskSlice("a", 10*time.Minute),
			taskSlice("b", 20*time.Minute),
		},
		Name:             "name",
		CreateTime:       timestamppb.New(testTime),
		Authenticated:    "user:authenticated",
		User:             "user",
		Tags:             []string{"tag1", "tag2"},
		ServiceAccount:   "service-account",
		Realm:            "realm",
		Priority:         123,
		BotPingTolerance: 456.0,
		PubsubNotification: &bqpb.PubSub{
			Topic:    "pubsub-topic",
			Userdata: "pubsub-user-data",
		},
		Resultdb: &apipb.ResultDBCfg{Enable: true},
	}
}

func TestTaskRequestConversion(t *testing.T) {
	t.Parallel()
	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	ftt.Run("Convert TaskRequest with empty parent task", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)
		sampleRequest := createTaskRequest(key, testTime)
		expected := createBQTaskRequest(taskID, testTime)
		actual := taskRequest(sampleRequest)
		assert.Loosely(t, actual, should.Resemble(expected))
	})

	ftt.Run("Converting empty EnvPrefixes works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		taskID := "65aba3a3e6b99310"
		key, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)
		sampleRequest := createTaskRequest(key, testTime)
		// Set this field to zero
		sampleRequest.TaskSlices[0].Properties.EnvPrefixes = make(model.EnvPrefixes)

		// Set this list to zero too
		expected := createBQTaskRequest(taskID, testTime)
		expected.TaskSlices[0].Properties.EnvPaths = make([]*bqpb.StringListPair, 0)
		actual := taskRequest(sampleRequest)
		assert.NoErr(t, err)
		assert.Loosely(t, actual, should.Resemble(expected))
	})
}

func TestPerformanceStatsConversion(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		got := performanceStats(createPerformanceStats(), 123.456)
		want := createBQPerformanceStats(123.456)
		assert.Loosely(t, got, should.Resemble(want))
	})
}

func TestTaskResultCommonConversion(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
	ctx := memory.Use(context.Background())

	taskID := "65aba3a3e6b99310"
	key, err := model.TaskIDToRequestKey(ctx, taskID)
	if err != nil {
		panic(err)
	}

	ftt.Run("Full", t, func(t *ftt.Test) {
		res := createTaskResultCommon(testTime)
		got := taskResultCommon(res, createTaskRequest(key, testTime))
		want := createBQTaskResultBase(taskID, testTime)
		assert.Loosely(t, got, should.Resemble(want))
	})

	ftt.Run("Internal error", t, func(t *ftt.Test) {
		res := createTaskResultCommon(testTime)
		res.InternalFailure = true
		got := taskResultCommon(res, createTaskRequest(key, testTime))

		want := createBQTaskResultBase(taskID, testTime)
		want.State = bqpb.TaskState_RAN_INTERNAL_FAILURE
		want.StateCategory = bqpb.TaskStateCategory_CATEGORY_TRANSIENT_DONE

		assert.Loosely(t, got, should.Resemble(want))
	})
}

func TestTaskResultSummaryConversion(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
	ctx := memory.Use(context.Background())

	taskID := "65aba3a3e6b99310"
	key, err := model.TaskIDToRequestKey(ctx, taskID)
	if err != nil {
		panic(err)
	}

	ftt.Run("Not dedupped", t, func(t *ftt.Test) {
		summary := &model.TaskResultSummary{
			Created:          testTime,
			TaskResultCommon: *createTaskResultCommon(testTime),
			TryNumber:        datastore.NewIndexedNullable(int64(1)),
			CostUSD:          123.456,
		}

		got := taskResultSummary(
			summary,
			createTaskRequest(key, testTime),
			createPerformanceStats(),
		)

		want := createBQTaskResultBase(taskID, testTime)
		want.RunId = taskID[:len(taskID)-1] + "1"
		want.TryNumber = 1
		want.Performance = createBQPerformanceStats(123.456)

		assert.Loosely(t, got, should.Resemble(want))
	})

	ftt.Run("Dedupped", t, func(t *ftt.Test) {
		const dedupedFrom = "65aba3a3e6b00001"

		summary := &model.TaskResultSummary{
			Created:          testTime,
			TaskResultCommon: *createTaskResultCommon(testTime),
			TryNumber:        datastore.NewIndexedNullable(int64(0)),
			DedupedFrom:      dedupedFrom,
			CostSavedUSD:     111.111,
		}

		got := taskResultSummary(
			summary,
			createTaskRequest(key, testTime),
			createPerformanceStats(),
		)

		want := createBQTaskResultBase(taskID, testTime)
		want.DedupedFrom = dedupedFrom
		want.RunId = dedupedFrom
		want.TryNumber = 0
		want.Performance = createBQPerformanceStats(0.0)
		want.State = bqpb.TaskState_DEDUPED
		want.StateCategory = bqpb.TaskStateCategory_CATEGORY_NEVER_RAN_DONE

		assert.Loosely(t, got, should.Resemble(want))
	})
}

func TestTaskRunResultConversion(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
	ctx := memory.Use(context.Background())

	taskID := "65aba3a3e6b99310"
	key, err := model.TaskIDToRequestKey(ctx, taskID)
	if err != nil {
		panic(err)
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		res := &model.TaskRunResult{
			TaskResultCommon: *createTaskResultCommon(testTime),
			CostUSD:          123.456,
		}

		got := taskRunResult(
			res,
			createTaskRequest(key, testTime),
			createPerformanceStats(),
		)

		want := createBQTaskResultBase(taskID, testTime)
		want.RunId = taskID[:len(taskID)-1] + "1"
		want.TryNumber = 1
		want.Performance = createBQPerformanceStats(123.456)

		assert.Loosely(t, got, should.Resemble(want))
	})
}

func TestBotEventConversion(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)
	eventTime := testTime.Add(time.Hour)
	ctx := memory.Use(context.Background())

	event := func(typ model.BotEventType, quarantined bool, maintenance, state string) *model.BotEvent {
		return &model.BotEvent{
			BotCommon: model.BotCommon{
				State:           botstate.Dict{JSON: []byte(state)},
				SessionID:       "test-session",
				ExternalIP:      "external-ip",
				AuthenticatedAs: "authenticated-as",
				Version:         "version",
				Quarantined:     quarantined,
				Maintenance:     maintenance,
				TaskID:          "task-id",
				LastSeen:        datastore.NewUnindexedOptional(testTime),
				IdleSince:       datastore.NewUnindexedOptional(testTime),
			},
			Key:       datastore.NewKey(ctx, "BotEvent", "", 12345, model.BotRootKey(ctx, "bot-id")),
			Timestamp: eventTime,
			EventType: typ,
			Message:   "Event message",
			Dimensions: []string{
				"id:bot-id",
				"pool:a",
				"pool:b",
				"stuff:x",
			},
		}
	}

	expected := func(typ bqpb.BotEventType, status bqpb.BotStatusType, msg, state string) *bqpb.BotEvent {
		var lastSeen *timestamppb.Timestamp
		if typ == bqpb.BotEventType_BOT_MISSING {
			lastSeen = timestamppb.New(testTime)
		}
		return &bqpb.BotEvent{
			EventTime: timestamppb.New(eventTime),
			Bot: &bqpb.Bot{
				BotId:         "bot-id",
				SessionId:     "test-session",
				Pools:         []string{"a", "b"},
				Status:        status,
				StatusMsg:     msg,
				CurrentTaskId: "task-id",
				Dimensions: []*bqpb.StringListPair{
					{Key: "id", Values: []string{"bot-id"}},
					{Key: "pool", Values: []string{"a", "b"}},
					{Key: "stuff", Values: []string{"x"}},
				},
				Info: &bqpb.BotInfo{
					Supplemental:    state,
					Version:         "version",
					ExternalIp:      "external-ip",
					AuthenticatedAs: "authenticated-as",
					IdleSinceTs:     timestamppb.New(testTime),
					LastSeenTs:      lastSeen,
				},
			},
			Event:    typ,
			EventMsg: "Event message",
		}
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t,
			botEvent(event(model.BotEventMissing, false, "", `{}`)),
			should.Resemble(
				expected(bqpb.BotEventType_BOT_MISSING, bqpb.BotStatusType_MISSING, "", `{}`),
			))

		assert.Loosely(t,
			botEvent(event(model.BotEventSleep, false, "", `{}`)),
			should.Resemble(
				expected(bqpb.BotEventType_INSTRUCT_IDLE, bqpb.BotStatusType_IDLE, "", `{}`),
			))

		state := `{"quarantined": "broke"}`
		assert.Loosely(t,
			botEvent(event(model.BotEventSleep, true, "ignored", state)),
			should.Resemble(
				expected(bqpb.BotEventType_INSTRUCT_IDLE, bqpb.BotStatusType_QUARANTINED_BY_BOT, "broke", state),
			))

		assert.Loosely(t,
			botEvent(event(model.BotEventSleep, false, "maintenance", `{}`)),
			should.Resemble(
				expected(bqpb.BotEventType_INSTRUCT_IDLE, bqpb.BotStatusType_OVERHEAD_MAINTENANCE_EXTERNAL, "maintenance", `{}`),
			))

		assert.Loosely(t,
			botEvent(event(model.BotEventTask, false, "", `{}`)),
			should.Resemble(
				expected(bqpb.BotEventType_INSTRUCT_START_TASK, bqpb.BotStatusType_BUSY, "", `{}`),
			))
	})
}
