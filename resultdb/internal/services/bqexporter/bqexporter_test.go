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
	"net/http"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	artifactcontenttest "go.chromium.org/luci/resultdb/internal/artifactcontent/testutil"
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

func (i *mockPassInserter) Put(ctx context.Context, src interface{}) error {
	messages := src.([]*bq.Row)
	i.mu.Lock()
	i.insertedMessages = append(i.insertedMessages, messages...)
	i.mu.Unlock()
	return nil
}

type mockFailInserter struct {
}

func (i *mockFailInserter) Put(ctx context.Context, src interface{}) error {
	return fmt.Errorf("some error")
}

func TestExportToBigQuery(t *testing.T) {
	Convey(`TestExportTestResultsToBigQuery`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		testutil.MustApply(ctx,
			insert.Invocation("a", pb.Invocation_FINALIZED, map[string]interface{}{"Realm": "testproject:testrealm"}),
			insert.Invocation("b", pb.Invocation_FINALIZED, map[string]interface{}{"Realm": "testproject:testrealm"}),
			insert.Inclusion("a", "b"))
		testutil.MustApply(ctx, testutil.CombineMutations(
			// Test results and exonerations have the same variants.
			insert.TestResults("a", "A", pbutil.Variant("k", "v"), pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insert.TestExonerations("a", "A", pbutil.Variant("k", "v"), 1),
			// Test results and exonerations have different variants.
			insert.TestResults("b", "B", pbutil.Variant("k", "v"), pb.TestStatus_CRASH, pb.TestStatus_PASS),
			insert.TestExonerations("b", "B", pbutil.Variant("k", "different"), 1),
			// Passing test result without exoneration.
			insert.TestResults("a", "C", nil, pb.TestStatus_PASS),
			// Test results' parent is different from exported.
			insert.TestResults("b", "D", pbutil.Variant("k", "v"), pb.TestStatus_CRASH, pb.TestStatus_PASS),
			insert.TestExonerations("b", "D", pbutil.Variant("k", "v"), 1),
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
			So(len(i.insertedMessages), ShouldEqual, 7)

			expectedTestIDs := []string{"A", "B", "C", "D"}
			for _, m := range i.insertedMessages {
				tr := m.Message.(*bqpb.TestResultRow)
				So(tr.TestId, ShouldBeIn, expectedTestIDs)
				So(tr.Parent.Id, ShouldBeIn, []string{"a", "b"})
				So(tr.Parent.Realm, ShouldEqual, "testproject:testrealm")
				So(tr.Exported.Id, ShouldEqual, "a")
				So(tr.Exported.Realm, ShouldEqual, "testproject:testrealm")
				So(tr.Exonerated, ShouldEqual, tr.TestId == "A" || tr.TestId == "D")
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
			insert.Invocation("a", pb.Invocation_FINALIZED, map[string]interface{}{"Realm": "testproject:testrealm"}),
			insert.Invocation("inv1", pb.Invocation_FINALIZED, map[string]interface{}{"Realm": "testproject:testrealm"}),
			insert.Inclusion("a", "inv1"),
			insert.Artifact("inv1", "", "a0", map[string]interface{}{"ContentType": "text/plain; encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/t/r", "a0", map[string]interface{}{"ContentType": "text/plain", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/t/r", "a1", nil),
			insert.Artifact("inv1", "tr/t/r", "a2", map[string]interface{}{"ContentType": "text/plain;encoding=ascii", "Size": "100", "RBECASHash": "deadbeef"}),
			insert.Artifact("inv1", "tr/t/r", "a3", map[string]interface{}{"ContentType": "image/jpg", "Size": "100"}),
			insert.Artifact("inv1", "tr/t/r", "a4", map[string]interface{}{"ContentType": "text/plain;encoding=utf-8", "Size": "100", "RBECASHash": "deadbeef"}),
		)

		bqExport := &pb.BigQueryExport{
			Project: "project",
			Dataset: "dataset",
			Table:   "table",
			ResultType: &pb.BigQueryExport_TextArtifacts_{
				TextArtifacts: &pb.BigQueryExport_TextArtifacts{
					Predicate: &pb.ArtifactPredicate{
						ContentTypeRegexp: "text/plain.*",
					},
				},
			},
		}

		opts := DefaultOptions()
		b := &bqExporter{
			Options:      &opts,
			putLimiter:   rate.NewLimiter(100, 1),
			batchSem:     semaphore.NewWeighted(100),
			rbecasClient: &artifactcontenttest.FakeByteStreamClient{bytes.Repeat([]byte("contentspart2\n"), 500000)},
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
}

type tableMock struct {
	fullyQualifiedName string

	md      *bigquery.TableMetadata
	mdCalls int
	mdErr   error

	createMD  *bigquery.TableMetadata
	createErr error

	updateMD  *bigquery.TableMetadataToUpdate
	updateErr error
}

func (t *tableMock) FullyQualifiedName() string {
	return t.fullyQualifiedName
}

func (t *tableMock) Metadata(ctx context.Context) (*bigquery.TableMetadata, error) {
	t.mdCalls++
	return t.md, t.mdErr
}

func (t *tableMock) Create(ctx context.Context, md *bigquery.TableMetadata) error {
	t.createMD = md
	return t.createErr
}

func (t *tableMock) Update(ctx context.Context, md bigquery.TableMetadataToUpdate, etag string) (*bigquery.TableMetadata, error) {
	t.updateMD = &md
	return t.md, t.updateErr
}

func TestBqTableCache(t *testing.T) {
	t.Parallel()
	Convey(`TestCheckBqTableCache`, t, func() {
		ctx := testutil.TestingContext()
		tc := clock.Get(ctx).(testclock.TestClock)
		ctx = caching.WithEmptyProcessCache(ctx)

		t := &tableMock{
			fullyQualifiedName: "project.dataset.table",
			md:                 &bigquery.TableMetadata{},
		}

		Convey(`Table does not exist`, func() {
			t.mdErr = &googleapi.Error{Code: http.StatusNotFound}
			err := ensureBQTable(ctx, t)
			So(err, ShouldBeNil)
			So(t.createMD.Schema, ShouldResemble, testResultRowSchema)
		})

		Convey(`Table is missing fields`, func() {
			t.md.Schema = bigquery.Schema{
				{
					Name: "legacy",
				},
				{
					Name:   "exported",
					Schema: bigquery.Schema{{Name: "legacy"}},
				},
			}
			err := ensureBQTable(ctx, t)
			So(err, ShouldBeNil)

			So(t.updateMD, ShouldNotBeNil) // The table was updated.
			So(len(t.updateMD.Schema), ShouldBeGreaterThan, 3)
			So(t.updateMD.Schema[0].Name, ShouldEqual, "legacy")
			So(t.updateMD.Schema[1].Name, ShouldEqual, "exported")
			So(t.updateMD.Schema[1].Schema[0].Name, ShouldEqual, "legacy")
			So(t.updateMD.Schema[1].Schema[1].Name, ShouldEqual, "id") // new field
			So(t.updateMD.Schema[1].Schema[1].Required, ShouldBeFalse) // relaxed
		})

		Convey(`Table is up to date`, func() {
			t.md.Schema = testResultRowSchema
			err := ensureBQTable(ctx, t)
			So(err, ShouldBeNil)
			So(t.updateMD, ShouldBeNil) // we did not try to update it
		})

		Convey(`Cache is working`, func() {
			err := ensureBQTable(ctx, t)
			So(err, ShouldBeNil)
			calls := t.mdCalls

			// Confirms the cache is working.
			err = ensureBQTable(ctx, t)
			So(err, ShouldBeNil)
			So(t.mdCalls, ShouldEqual, calls) // no more new calls were made.

			// Confirms the cache is expired as expected.
			tc.Add(6 * time.Minute)
			err = ensureBQTable(ctx, t)
			So(err, ShouldBeNil)
			So(t.mdCalls, ShouldBeGreaterThan, calls) // new calls were made.
		})
	})
}

func TestSchedule(t *testing.T) {
	Convey(`TestSchedule`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		bqx1 := &pb.BigQueryExport{Dataset: "dataset", Project: "project", Table: "table"}
		bqx2 := &pb.BigQueryExport{Dataset: "dataset2", Project: "project2", Table: "table2"}
		bqx1Bytes, _ := proto.Marshal(bqx1)
		bqx2Bytes, _ := proto.Marshal(bqx2)
		exports := [][]byte{bqx1Bytes, bqx2Bytes}
		testutil.MustApply(ctx,
			insert.Invocation("two-bqx", pb.Invocation_FINALIZED, map[string]interface{}{"BigqueryExports": exports}),
			insert.Invocation("one-bqx", pb.Invocation_FINALIZED, map[string]interface{}{"BigqueryExports": exports[:1]}),
			insert.Invocation("zero-bqx", pb.Invocation_FINALIZED, nil))

		ctx, sched := tq.TestingContext(ctx, nil)
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			So(Schedule(ctx, "two-bqx"), ShouldBeNil)
			So(Schedule(ctx, "one-bqx"), ShouldBeNil)
			So(Schedule(ctx, "zero-bqx"), ShouldBeNil)
			return nil
		})
		So(err, ShouldBeNil)
		So(sched.Tasks().Payloads(), ShouldResembleProto, []*taskspb.ExportInvocationToBQ{
			{InvocationId: "one-bqx", BqExport: bqx1},
			{InvocationId: "two-bqx", BqExport: bqx2},
			{InvocationId: "two-bqx", BqExport: bqx1},
		})
	})
}
