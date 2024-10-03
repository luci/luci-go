// Copyright 2024 The LUCI Authors.
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

package resultingester

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerrit"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"

	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

func TestSchedule(t *testing.T) {
	ftt.Run(`TestSchedule`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)

		task := &taskspb.IngestTestResults{
			Notification: &rdbpb.InvocationReadyForExportNotification{
				RootInvocation:      "invocations/build-123456",
				RootInvocationRealm: "testproject:ci",
				Invocation:          "invocations/build-987654",
				InvocationRealm:     "testproject:test_runner",
				Sources: &rdbpb.Sources{
					GitilesCommit: &rdbpb.GitilesCommit{
						Host:       "testproject.googlesource.com",
						Project:    "testproject/src",
						Ref:        "refs/heads/main",
						CommitHash: "1234567890123456789012345678901234567890",
						Position:   123,
					},
				},
			},
			PageToken: "",
			TaskIndex: 1,
		}
		expected := proto.Clone(task).(*taskspb.IngestTestResults)

		Schedule(ctx, task)

		assert.Loosely(t, skdr.Tasks().Payloads()[0], should.Resemble(expected))
	})
}

func TestOrchestrator(t *testing.T) {
	ftt.Run(`TestOrchestrator`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, skdr := tq.TestingContext(ctx, nil)
		ctx = memory.Use(ctx)

		ctl := gomock.NewController(t)
		defer ctl.Finish()

		mrc := resultdb.NewMockedClient(ctx, ctl)
		mbc := buildbucket.NewMockedClient(mrc.Ctx, ctl)
		ctx = mbc.Ctx

		clsByHost := gerritChangesByHostForTesting()
		ctx = gerrit.UseFakeClient(ctx, clsByHost)

		setupGetParentInvocationMock := func() {
			invReq := &rdbpb.GetInvocationRequest{
				Name: "invocations/test-invocation-name",
			}
			invRes := resultdbParentInvocationForTesting()
			mrc.GetInvocation(invReq, invRes)
		}

		setupQueryRunTestVariantsMock := func(modifiers ...func(*rdbpb.QueryRunTestVerdictsResponse)) {
			tvReq := &rdbpb.QueryRunTestVerdictsRequest{
				Invocation:  "invocations/test-invocation-name",
				PageSize:    10000,
				ResultLimit: 100,
				PageToken:   "expected_token",
			}
			tvRsp := mockedQueryRunTestVerdictsRsp()
			tvRsp.NextPageToken = "continuation_token"
			for _, modifier := range modifiers {
				modifier(tvRsp)
			}
			mrc.QueryRunTestVerdicts(tvReq, tvRsp)
		}

		testIngestor := &testIngester{}

		o := &orchestrator{}
		o.sinks = []IngestionSink{
			testIngestor,
		}

		notification := &rdbpb.InvocationReadyForExportNotification{
			ResultdbHost:        "fake.rdb.host",
			Invocation:          "invocations/test-invocation-name",
			InvocationRealm:     "invproject:inv",
			RootInvocation:      "invocations/test-root-invocation-name",
			RootInvocationRealm: "rootproject:root",
			RootCreateTime:      timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
			Sources:             resultdbSourcesForTesting(),
		}

		expectedInputs := Inputs{
			Project:          "rootproject",
			SubRealm:         "root",
			ResultDBHost:     "fake.rdb.host",
			RootInvocationID: "test-root-invocation-name",
			InvocationID:     "test-invocation-name",
			PageNumber:       1,
			PartitionTime:    time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC),
			Sources:          resolvedSourcesForTesting(),
			Parent:           resultdbParentInvocationForTesting(),
			Verdicts:         mockedQueryRunTestVerdictsRsp().RunTestVerdicts,
		}

		task := &taskspb.IngestTestResults{
			Notification: notification,
			PageToken:    "expected_token",
			TaskIndex:    1,
		}
		expectedContinuationTask := &taskspb.IngestTestResults{
			Notification: notification,
			PageToken:    "continuation_token",
			TaskIndex:    2,
		}

		expectedCheckpoint := checkpoints.Checkpoint{
			Key: checkpoints.Key{
				Project:    "rootproject",
				ResourceID: "fake.rdb.host/test-root-invocation-name/test-invocation-name",
				ProcessID:  "result-ingestion/schedule-continuation",
				Uniquifier: "1",
			},
			// Creation and expiry time not validated.
		}

		cfg := &configpb.Config{}
		err := config.SetTestConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		t.Run(`Baseline`, func(t *ftt.Test) {
			setupGetParentInvocationMock()
			setupQueryRunTestVariantsMock()
			err := o.run(ctx, task)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeTrue)
			assert.Loosely(t, testIngestor.gotInputs, should.Resemble(expectedInputs))

			// Expect continuation task.
			verifyContinuationTask(t, skdr, expectedContinuationTask)
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoint), should.BeNil)
		})
		t.Run(`Without sources`, func(t *ftt.Test) {
			notification.Sources = nil
			expectedInputs.Sources = nil

			setupGetParentInvocationMock()
			setupQueryRunTestVariantsMock()
			err := o.run(ctx, task)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeTrue)
			assert.Loosely(t, testIngestor.gotInputs, should.Resemble(expectedInputs))

			// Expect continuation task.
			verifyContinuationTask(t, skdr, expectedContinuationTask)
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoint), should.BeNil)
		})
		t.Run(`Continuation task previously scheduled`, func(t *ftt.Test) {
			// Create a checkpoint for the previous scheduling
			// of the continuation task.
			err := checkpoints.SetForTesting(ctx, t, expectedCheckpoint)
			assert.Loosely(t, err, should.BeNil)

			setupGetParentInvocationMock()
			setupQueryRunTestVariantsMock()
			err = o.run(ctx, task)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeTrue)
			assert.Loosely(t, testIngestor.gotInputs, should.Resemble(expectedInputs))

			// Expect no further continuation task.
			verifyContinuationTask(t, skdr, nil)
			assert.Loosely(t, verifyCheckpoints(ctx, t, expectedCheckpoint), should.BeNil)
		})
		t.Run(`Final page of results`, func(t *ftt.Test) {
			setupGetParentInvocationMock()
			setupQueryRunTestVariantsMock(func(qrtvr *rdbpb.QueryRunTestVerdictsResponse) {
				qrtvr.NextPageToken = ""
			})
			err := o.run(ctx, task)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeTrue)
			assert.Loosely(t, testIngestor.gotInputs, should.Resemble(expectedInputs))

			// Expect no continuation task.
			verifyContinuationTask(t, skdr, nil)
			// Expect no checkpoint.
			assert.Loosely(t, verifyCheckpoints(ctx, t), should.BeNil)
		})
		t.Run(`Project not allowlisted for ingestion`, func(t *ftt.Test) {
			cfg.Ingestion = &configpb.Ingestion{
				ProjectAllowlistEnabled: true,
				ProjectAllowlist:        []string{"other"},
			}
			err := config.SetTestConfig(ctx, cfg)
			assert.Loosely(t, err, should.BeNil)

			err = o.run(ctx, task)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, testIngestor.called, should.BeFalse)
		})
	})
}

type testIngester struct {
	called    bool
	gotInputs Inputs
}

func (t *testIngester) Name() string {
	return "test-ingestor"
}

func (t *testIngester) Ingest(ctx context.Context, inputs Inputs) error {
	if t.called {
		return errors.New("already called")
	}
	t.gotInputs = inputs
	t.called = true
	return nil
}

func verifyContinuationTask(t testing.TB, skdr *tqtesting.Scheduler, expectedContinuation *taskspb.IngestTestResults) {
	t.Helper()
	count := 0
	for _, pl := range skdr.Tasks().Payloads() {
		if pl, ok := pl.(*taskspb.IngestTestResults); ok {
			assert.Loosely(t, pl, should.Resemble(expectedContinuation), truth.LineContext())
			count++
		}
	}
	if expectedContinuation != nil {
		assert.Loosely(t, count, should.Equal(1), truth.LineContext())
	} else {
		assert.Loosely(t, count, should.BeZero, truth.LineContext())
	}
}

func verifyCheckpoints(ctx context.Context, t testing.TB, expected ...checkpoints.Checkpoint) error {
	t.Helper()
	result, err := checkpoints.ReadAllForTesting(span.Single(ctx))
	if err != nil {
		return err
	}

	var wantKeys []checkpoints.Key
	var gotKeys []checkpoints.Key
	for _, c := range expected {
		wantKeys = append(wantKeys, c.Key)
	}
	for _, c := range result {
		gotKeys = append(gotKeys, c.Key)
	}
	assert.That(t, gotKeys, should.Match(wantKeys), truth.LineContext())
	return nil
}
