// Copyright 2019 The LUCI Authors.
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

package bqexporter

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type mockPassInserter struct {
	insertedMessages []*bq.Row
	mu               sync.Mutex
}

func (i *mockPassInserter) Put(ctx context.Context, src any) error {
	messages := src.([]*bq.Row)
	i.mu.Lock()
	i.insertedMessages = append(i.insertedMessages, messages...)
	i.mu.Unlock()
	return nil
}

type mockFailInserter struct {
}

func (i *mockFailInserter) Put(ctx context.Context, src any) error {
	return fmt.Errorf("some error")
}

func TestExportToBigQuery(t *testing.T) {
	Convey(`TestExportTestResultsToBigQuery`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
		testutil.MustApply(ctx,
			insert.Invocation("a", pb.Invocation_FINALIZED, map[string]any{
				"Realm":   "testproject:testrealm",
				"Sources": spanutil.Compressed(pbutil.MustMarshal(testutil.TestSources())),
			}),
			insert.Invocation("b", pb.Invocation_FINALIZED, map[string]any{
				"Realm": "testproject:testrealm",
				"Properties": spanutil.Compressed(pbutil.MustMarshal(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"key": structpb.NewStringValue("value"),
					},
				})),
				"InheritSources": true,
			}),
			insert.Inclusion("a", "b"))
		testutil.MustApply(ctx, testutil.CombineMutations(
			// Test results and exonerations have the same variants.
			insert.TestResults("a", "A", pbutil.Variant("k", "v"), pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insert.TestExonerations("a", "A", pbutil.Variant("k", "v"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			// Test results and exonerations have different variants.
			insert.TestResults("b", "B", pbutil.Variant("k", "v"), pb.TestStatus_CRASH, pb.TestStatus_PASS),
			insert.TestExonerations("b", "B", pbutil.Variant("k", "different"), pb.ExonerationReason_OCCURS_ON_MAINLINE),
			// Passing test result without exoneration.
			insert.TestResults("a", "C", nil, pb.TestStatus_PASS),
			// Test results' parent is different from exported.
			insert.TestResults("b", "D", pbutil.Variant("k", "v"), pb.TestStatus_CRASH, pb.TestStatus_PASS),
			insert.TestExonerations("b", "D", pbutil.Variant("k", "v"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			insert.TestResultMessages([]*pb.TestResult{
				{
					Name:        pbutil.TestResultName("a", "E", "0"),
					TestId:      "E",
					ResultId:    "0",
					Variant:     pbutil.Variant("k2", "v2", "k3", "v3"),
					VariantHash: pbutil.VariantHash(pbutil.Variant("k2", "v2", "k3", "v3")),
					Expected:    true,
					Status:      pb.TestStatus_SKIP,
					SkipReason:  pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS,
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
							"key_2": structpb.NewStructValue(&structpb.Struct{
								Fields: map[string]*structpb.Value{
									"child_key": structpb.NewNumberValue(1),
								},
							}),
						},
					},
				},
			}),
		)...)

		bqExport := &pb.BigQueryExport{
			Project: "project",
			Dataset: "dataset",
			Table:   "table",
			ResultType: &pb.BigQueryExport_TestResults_{
				TestResults: &pb.BigQueryExport_TestResults{},
			},
		}

		opts := DefaultOptions()
		b := &bqExporter{
			Options:    &opts,
			putLimiter: rate.NewLimiter(100, 1),
			batchSem:   semaphore.NewWeighted(100),
		}

		Convey(`success`, func() {
			i := &mockPassInserter{}
			err := b.exportTestResultsToBigQuery(ctx, i, "a", bqExport)
			So(err, ShouldBeNil)

			i.mu.Lock()
			defer i.mu.Unlock()
			So(len(i.insertedMessages), ShouldEqual, 8)

			expectedTestIDs := []string{"A", "B", "C", "D", "E"}
			for _, m := range i.insertedMessages {
				tr := m.Message.(*bqpb.TestResultRow)
				So(tr.TestId, ShouldBeIn, expectedTestIDs)
				So(tr.Parent.Id, ShouldBeIn, []string{"a", "b"})
				So(tr.Parent.Realm, ShouldEqual, "testproject:testrealm")
				if tr.Parent.Id == "b" {
					So(tr.Parent.Properties, ShouldResembleProto, &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("value"),
						},
					})
				} else {
					So(tr.Parent.Properties, ShouldBeNil)
				}

				So(tr.Exported.Id, ShouldEqual, "a")
				So(tr.Exported.Realm, ShouldEqual, "testproject:testrealm")
				So(tr.Exported.Properties, ShouldBeNil)
				So(tr.Exonerated, ShouldEqual, tr.TestId == "A" || tr.TestId == "D")

				So(tr.Name, ShouldEqual, pbutil.TestResultName(string(tr.Parent.Id), tr.TestId, tr.ResultId))
				So(tr.InsertTime, ShouldEqual, timestamppb.New(testclock.TestTimeUTC))

				if tr.TestId == "E" {
					So(tr.Properties, ShouldResembleProto, &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
							"key_2": structpb.NewStructValue(&structpb.Struct{
								Fields: map[string]*structpb.Value{
									"child_key": structpb.NewNumberValue(1),
								},
							}),
						},
					})
					So(tr.SkipReason, ShouldEqual, pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS.String())
				} else {
					So(tr.Properties, ShouldResembleProto, &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("value"),
						},
					})
					So(tr.SkipReason, ShouldBeEmpty)
				}

				So(tr.Sources, ShouldResembleProto, testutil.TestSources())
			}
		})

		// To check when encountering an error, the test can run to the end
		// without hanging, or race detector does not detect anything.
		Convey(`fail`, func() {
			err := b.exportTestResultsToBigQuery(ctx, &mockFailInserter{}, "a", bqExport)
			So(err, ShouldErrLike, "some error")
		})
	})

	Convey(`TestExportTextArtifactToBigQuery`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		testutil.MustApply(ctx,
			insert.Invocation("a", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Invocation("inv1", pb.Invocation_FINALIZED, map[string]any{"Realm": "testproject:testrealm"}),
			insert.Inclusion("a", "inv1"),
			insert.Artifact("inv1", "", "a0", map[string]any{"ContentType": "text/plain; encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/t/r", "a0", map[string]any{"ContentType": "text/plain", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/t/r", "a1", nil),
			insert.Artifact("inv1", "tr/t/r", "a2", map[string]any{"ContentType": "text/plain;encoding=ascii", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/t/r", "a3", map[string]any{"ContentType": "image/jpg", "Size": "100"}),
			insert.Artifact("inv1", "tr/t/r", "a4", map[string]any{"ContentType": "text/plain;encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
		)

		bqExport := &pb.BigQueryExport{
			Project: "project",
			Dataset: "dataset",
			Table:   "table",
			ResultType: &pb.BigQueryExport_TextArtifacts_{
				TextArtifacts: &pb.BigQueryExport_TextArtifacts{
					Predicate: &pb.ArtifactPredicate{},
				},
			},
		}

		opts := DefaultOptions()
		b := &bqExporter{
			Options:    &opts,
			putLimiter: rate.NewLimiter(100, 1),
			batchSem:   semaphore.NewWeighted(100),
			rbecasClient: &artifactcontenttest.FakeByteStreamClient{
				ExtraResponseData: bytes.Repeat([]byte("short\ncontentspart2\n"), 200000),
			},
			maxTokenSize: 10,
		}

		Convey(`success`, func() {
			i := &mockPassInserter{}
			err := b.exportTextArtifactsToBigQuery(ctx, i, "a", bqExport)
			So(err, ShouldBeNil)

			i.mu.Lock()
			defer i.mu.Unlock()
			So(len(i.insertedMessages), ShouldEqual, 8)
		})

		Convey(`fail`, func() {
			err := b.exportTextArtifactsToBigQuery(ctx, &mockFailInserter{}, "a", bqExport)
			So(err, ShouldErrLike, "some error")
		})
	})

	Convey(`TestExportInvocationToBigQuery`, t, func() {
		// TODO(crbug.com/341362001): Add the test after implementing the export
	})
}

func TestSchedule(t *testing.T) {
	Convey(`TestSchedule`, t, func() {
		// TODO(crbug.com/341362001): Add the test after implementing the export
		ctx := testutil.SpannerTestContext(t)
		bqExport1 := &pb.BigQueryExport{Dataset: "dataset", Project: "project", Table: "table", ResultType: &pb.BigQueryExport_TestResults_{}}
		bqExport2 := &pb.BigQueryExport{Dataset: "dataset2", Project: "project2", Table: "table2", ResultType: &pb.BigQueryExport_TextArtifacts_{}}
		bqExports := []*pb.BigQueryExport{bqExport1, bqExport2}
		testutil.MustApply(ctx,
			insert.Invocation("two-bqx", pb.Invocation_FINALIZED, map[string]any{"BigqueryExports": bqExports}),
			insert.Invocation("one-bqx", pb.Invocation_FINALIZED, map[string]any{"BigqueryExports": bqExports[:1]}),
			insert.Invocation("zero-bqx", pb.Invocation_FINALIZED, nil))

		ctx, sched := tq.TestingContext(ctx, nil)
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			So(Schedule(ctx, "two-bqx"), ShouldBeNil)
			So(Schedule(ctx, "one-bqx"), ShouldBeNil)
			So(Schedule(ctx, "zero-bqx"), ShouldBeNil)
			return nil
		})
		So(err, ShouldBeNil)
		So(sched.Tasks().Payloads()[0], ShouldResembleProto, &taskspb.ExportInvocationTestResultsToBQ{InvocationId: "one-bqx", BqExport: bqExport1})
		So(sched.Tasks().Payloads()[1], ShouldResembleProto, &taskspb.ExportInvocationArtifactsToBQ{InvocationId: "two-bqx", BqExport: bqExport2})
		So(sched.Tasks().Payloads()[2], ShouldResembleProto, &taskspb.ExportInvocationTestResultsToBQ{InvocationId: "two-bqx", BqExport: bqExport1})
	})
}
