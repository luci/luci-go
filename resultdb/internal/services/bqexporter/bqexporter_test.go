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
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq"
	pb "go.chromium.org/luci/resultdb/proto/v1"
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

type fakeInvClient struct {
	Rows  []*bqpb.InvocationRow
	Error error
}

func (c *fakeInvClient) InsertInvocationRow(ctx context.Context, row *bqpb.InvocationRow) error {
	if c.Error != nil {
		return c.Error
	}
	c.Rows = append(c.Rows, row)
	return nil
}

func TestExportToBigQuery(t *testing.T) {
	ftt.Run(`TestExportTestResultsToBigQuery`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, _ = testclock.UseTime(ctx, testclock.TestTimeUTC)
		testutil.MustApply(ctx, t,
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
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			// Test results and exonerations have the same variants.
			insert.TestResults(t, "a", "A", pbutil.Variant("k", "v"), pb.TestResult_FAILED, pb.TestResult_PASSED),
			insert.TestExonerations("a", "A", pbutil.Variant("k", "v"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			// Test results and exonerations have different variants.
			insert.TestResults(t, "b", "B", pbutil.Variant("k", "v"), pb.TestResult_FAILED, pb.TestResult_PASSED),
			insert.TestExonerations("b", "B", pbutil.Variant("k", "different"), pb.ExonerationReason_OCCURS_ON_MAINLINE),
			// Passing test result without exoneration.
			insert.TestResults(t, "a", "C", nil, pb.TestResult_PASSED),
			// Test results' parent is different from exported.
			insert.TestResults(t, "b", "D", pbutil.Variant("k", "v"), pb.TestResult_FAILED, pb.TestResult_PASSED),
			insert.TestExonerations("b", "D", pbutil.Variant("k", "v"), pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
			insert.TestResultMessages(t, []*pb.TestResult{
				{
					Name:        pbutil.LegacyTestResultName("a", "E", "0"),
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

		t.Run(`success`, func(t *ftt.Test) {
			i := &mockPassInserter{}
			err := b.exportTestResultsToBigQuery(ctx, i, "a", bqExport)
			assert.Loosely(t, err, should.BeNil)

			i.mu.Lock()
			defer i.mu.Unlock()
			assert.Loosely(t, len(i.insertedMessages), should.Equal(8))

			expectedTestIDs := []string{"A", "B", "C", "D", "E"}
			for _, m := range i.insertedMessages {
				tr := m.Message.(*bqpb.TestResultRow)
				assert.Loosely(t, tr.TestId, should.BeIn(expectedTestIDs...))
				assert.Loosely(t, tr.Parent.Id, should.BeIn([]string{"a", "b"}...))
				assert.Loosely(t, tr.Parent.Realm, should.Equal("testproject:testrealm"))
				if tr.Parent.Id == "b" {
					assert.Loosely(t, tr.Parent.Properties, should.Match(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("value"),
						},
					}))
				} else {
					assert.Loosely(t, tr.Parent.Properties, should.BeNil)
				}

				assert.Loosely(t, tr.Exported.Id, should.Equal("a"))
				assert.Loosely(t, tr.Exported.Realm, should.Equal("testproject:testrealm"))
				assert.Loosely(t, tr.Exported.Properties, should.BeNil)
				assert.Loosely(t, tr.Exonerated, should.Equal(tr.TestId == "A" || tr.TestId == "D"))

				assert.Loosely(t, tr.Name, should.Equal(pbutil.LegacyTestResultName(string(tr.Parent.Id), tr.TestId, tr.ResultId)))
				assert.Loosely(t, tr.InsertTime, should.Match(timestamppb.New(testclock.TestTimeUTC)))

				if tr.TestId == "E" {
					assert.Loosely(t, tr.Properties, should.Match(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key_1": structpb.NewStringValue("value_1"),
							"key_2": structpb.NewStructValue(&structpb.Struct{
								Fields: map[string]*structpb.Value{
									"child_key": structpb.NewNumberValue(1),
								},
							}),
						},
					}))
					assert.Loosely(t, tr.SkipReason, should.Equal(pb.SkipReason_AUTOMATICALLY_DISABLED_FOR_FLAKINESS.String()))
				} else {
					assert.Loosely(t, tr.Properties, should.Match(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"key": structpb.NewStringValue("value"),
						},
					}))
					assert.Loosely(t, tr.SkipReason, should.BeEmpty)
				}

				assert.Loosely(t, tr.Sources, should.Match(testutil.TestSources()))
			}
		})

		// To check when encountering an error, the test can run to the end
		// without hanging, or race detector does not detect anything.
		t.Run(`fail`, func(t *ftt.Test) {
			err := b.exportTestResultsToBigQuery(ctx, &mockFailInserter{}, "a", bqExport)
			assert.Loosely(t, err, should.ErrLike("some error"))
		})
	})

	ftt.Run(`TestExportTextArtifactToBigQuery`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		testutil.MustApply(ctx, t,
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

		t.Run(`success`, func(t *ftt.Test) {
			i := &mockPassInserter{}
			err := b.exportTextArtifactsToBigQuery(ctx, i, "a", bqExport)
			assert.Loosely(t, err, should.BeNil)

			i.mu.Lock()
			defer i.mu.Unlock()
			assert.Loosely(t, len(i.insertedMessages), should.Equal(8))
		})

		t.Run(`fail`, func(t *ftt.Test) {
			err := b.exportTextArtifactsToBigQuery(ctx, &mockFailInserter{}, "a", bqExport)
			assert.Loosely(t, err, should.ErrLike("some error"))
		})
	})

	ftt.Run(`TestExportInvocationToBigQuery`, t, func(t *ftt.Test) {
		properties, err := structpb.NewStruct(map[string]interface{}{
			"num_prop":    123,
			"string_prop": "ABC",
		})
		assert.Loosely(t, err, should.BeNil)
		extendedProperties := map[string]*structpb.Struct{
			"a_key": properties,
		}
		internalExtendedProperties := &invocationspb.ExtendedProperties{
			ExtendedProperties: extendedProperties,
		}
		ctx := testutil.SpannerTestContext(t)
		commitTime := testutil.MustApply(ctx, t,
			insert.Invocation("a", pb.Invocation_FINALIZED, map[string]any{
				"Realm":              "testproject:testrealm",
				"Properties":         spanutil.Compressed(pbutil.MustMarshal(properties)),
				"ExtendedProperties": spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties)),
			}),
			insert.Invocation("b", pb.Invocation_ACTIVE, map[string]any{
				"Realm": "testproject:testrealm",
			}))

		opts := DefaultOptions()
		b := &bqExporter{
			Options:    &opts,
			putLimiter: rate.NewLimiter(100, 1),
			batchSem:   semaphore.NewWeighted(100),
		}

		t.Run("Invocation not finalized", func(t *ftt.Test) {
			invClient := &fakeInvClient{}
			err = b.exportInvocationToBigQuery(ctx, "b", invClient)
			assert.Loosely(t, err, should.ErrLike("invocations/b not finalized"))
		})

		t.Run("Export failed", func(t *ftt.Test) {
			invClient := &fakeInvClient{
				Error: errors.New("bq error"),
			}
			err = b.exportInvocationToBigQuery(ctx, "a", invClient)
			assert.Loosely(t, err, should.ErrLike("bq error"))
		})

		t.Run("Succeed", func(t *ftt.Test) {
			invClient := &fakeInvClient{}
			err = b.exportInvocationToBigQuery(ctx, "a", invClient)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(invClient.Rows), should.Equal(1))
			row := invClient.Rows[0]
			assert.Loosely(t, row.Project, should.Equal("testproject"))
			assert.Loosely(t, row.Realm, should.Equal("testrealm"))
			assert.Loosely(t, row.Id, should.Equal("a"))
			assert.Loosely(t, row.CreateTime, should.Match(timestamppb.New(commitTime)))
			assert.Loosely(t, row.FinalizeTime, should.Match(timestamppb.New(commitTime)))
			assert.Loosely(t, row.PartitionTime, should.Match(timestamppb.New(commitTime)))

			// Different implementations may use different spacing between
			// json elements. Ignore this.
			rowProp := strings.ReplaceAll(row.Properties, " ", "")
			assert.Loosely(t, rowProp, should.Match(`{"num_prop":123,"string_prop":"ABC"}`))
			rowExtProp := strings.ReplaceAll(row.ExtendedProperties, " ", "")
			assert.Loosely(t, rowExtProp, should.Match(`{"a_key":{"num_prop":123,"string_prop":"ABC"}}`))
		})
	})
}

func TestSchedule(t *testing.T) {
	ftt.Run(`TestSchedule`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		bqExport1 := &pb.BigQueryExport{Dataset: "dataset", Project: "project", Table: "table", ResultType: &pb.BigQueryExport_TestResults_{}}
		bqExport2 := &pb.BigQueryExport{Dataset: "dataset2", Project: "project2", Table: "table2", ResultType: &pb.BigQueryExport_TextArtifacts_{}}
		bqExports := []*pb.BigQueryExport{bqExport1, bqExport2}
		testutil.MustApply(ctx, t,
			insert.Invocation("two-bqx", pb.Invocation_FINALIZED, map[string]any{"BigqueryExports": bqExports}),
			insert.Invocation("one-bqx", pb.Invocation_FINALIZED, map[string]any{"BigqueryExports": bqExports[:1]}),
			insert.Invocation("zero-bqx", pb.Invocation_FINALIZED, nil))

		ctx, sched := tq.TestingContext(ctx, nil)
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			assert.Loosely(t, Schedule(ctx, "two-bqx"), should.BeNil)
			assert.Loosely(t, Schedule(ctx, "one-bqx"), should.BeNil)
			assert.Loosely(t, Schedule(ctx, "zero-bqx"), should.BeNil)
			return nil
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sched.Tasks().Payloads()[0], should.Match(&taskspb.ExportInvocationToBQ{InvocationId: "zero-bqx"}))
		assert.Loosely(t, sched.Tasks().Payloads()[1], should.Match(&taskspb.ExportInvocationToBQ{InvocationId: "one-bqx"}))
		assert.Loosely(t, sched.Tasks().Payloads()[2], should.Match(&taskspb.ExportInvocationTestResultsToBQ{InvocationId: "one-bqx", BqExport: bqExport1}))
		assert.Loosely(t, sched.Tasks().Payloads()[3], should.Match(&taskspb.ExportInvocationToBQ{InvocationId: "two-bqx"}))
		assert.Loosely(t, sched.Tasks().Payloads()[4], should.Match(&taskspb.ExportInvocationArtifactsToBQ{InvocationId: "two-bqx", BqExport: bqExport2}))
		assert.Loosely(t, sched.Tasks().Payloads()[5], should.Match(&taskspb.ExportInvocationTestResultsToBQ{InvocationId: "two-bqx", BqExport: bqExport1}))
	})
}
