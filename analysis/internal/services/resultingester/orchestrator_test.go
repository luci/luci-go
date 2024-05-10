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

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/buildbucket"
	"go.chromium.org/luci/analysis/internal/gerrit"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"

	_ "go.chromium.org/luci/server/tq/txn/spanner"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSchedule(t *testing.T) {
	Convey(`TestSchedule`, t, func() {
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

		So(skdr.Tasks().Payloads()[0], ShouldResembleProto, expected)
	})
}

func TestOrchestrator(t *testing.T) {
	Convey(`TestOrchestrator`, t, func() {
		ctx := testutil.IntegrationTestContext(t)

		ctl := gomock.NewController(t)
		defer ctl.Finish()

		mrc := resultdb.NewMockedClient(ctx, ctl)
		mbc := buildbucket.NewMockedClient(mrc.Ctx, ctl)
		ctx = mbc.Ctx

		clsByHost := gerritChangesByHostForTesting()
		ctx = gerrit.UseFakeClient(ctx, clsByHost)

		setupGetInvocationMock := func() {
			invReq := &rdbpb.GetInvocationRequest{
				Name: "invocations/test-root-invocation-name",
			}
			invRes := &rdbpb.Invocation{
				Name:       "invocations/test-root-invocation-name",
				Realm:      "rootproject:root",
				CreateTime: timestamppb.New(time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC)),
			}
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
			ResultdbHost:        "fake-host",
			Invocation:          "invocations/test-invocation-name",
			InvocationRealm:     "invproject:inv",
			RootInvocation:      "invocations/test-root-invocation-name",
			RootInvocationRealm: "rootproject:root",
			Sources:             resultdbSourcesForTesting(),
		}

		expectedInputs := Inputs{
			Project:          "rootproject",
			SubRealm:         "root",
			RootInvocationID: "test-root-invocation-name",
			InvocationID:     "test-invocation-name",
			PartitionTime:    time.Date(2020, 2, 3, 4, 5, 6, 7, time.UTC),
			Sources:          resolvedSourcesForTesting(),
			Verdicts:         mockedQueryRunTestVerdictsRsp().RunTestVerdicts,
		}

		Convey(`With sources`, func() {
			setupGetInvocationMock()
			setupQueryRunTestVariantsMock()
			err := o.run(ctx, &taskspb.IngestTestResults{
				Notification: notification,
				PageToken:    "expected_token",
			})
			So(err, ShouldBeNil)

			So(testIngestor.called, ShouldBeTrue)
			So(testIngestor.gotInputs, ShouldResembleProto, expectedInputs)
		})
		Convey(`Without sources`, func() {
			notification.Sources = nil
			expectedInputs.Sources = nil

			setupGetInvocationMock()
			setupQueryRunTestVariantsMock()
			err := o.run(ctx, &taskspb.IngestTestResults{
				Notification: notification,
				PageToken:    "expected_token",
			})
			So(err, ShouldBeNil)

			So(testIngestor.called, ShouldBeTrue)
			So(testIngestor.gotInputs, ShouldResembleProto, expectedInputs)
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
