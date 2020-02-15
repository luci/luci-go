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

package backend

import (
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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type mockPassInserter struct {
	insertedMessages []*bigquery.StructSaver
	mu               sync.Mutex
}

func (i *mockPassInserter) Put(ctx context.Context, src interface{}) error {
	messages := src.([]*bigquery.StructSaver)
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
			testutil.InsertInvocation("a", pb.Invocation_FINALIZED, nil),
			testutil.InsertInvocation("b", pb.Invocation_FINALIZED, nil),
			testutil.InsertInclusion("a", "b"))
		testutil.MustApply(ctx, testutil.CombineMutations(
			// Test results and exonerations have the same variants.
			testutil.InsertTestResults(testutil.MakeTestResults("a", "A", pbutil.Variant("k", "v"), pb.TestStatus_FAIL, pb.TestStatus_PASS)),
			testutil.InsertTestExonerations("a", "A", pbutil.Variant("k", "v"), 1),
			// Test results and exonerations have different variants.
			testutil.InsertTestResults(testutil.MakeTestResults("b", "B", pbutil.Variant("k", "v"), pb.TestStatus_CRASH, pb.TestStatus_PASS)),
			testutil.InsertTestExonerations("b", "B", pbutil.Variant("k", "different"), 1),
			// Passing test result without exoneration.
			testutil.InsertTestResults(testutil.MakeTestResults("a", "C", nil, pb.TestStatus_PASS)),
			// Test results' parent is different from exported.
			testutil.InsertTestResults(testutil.MakeTestResults("b", "D", pbutil.Variant("k", "v"), pb.TestStatus_CRASH, pb.TestStatus_PASS)),
			testutil.InsertTestExonerations("b", "D", pbutil.Variant("k", "v"), 1),
		)...)

		bqExport := &pb.BigQueryExport{
			Project:     "project",
			Dataset:     "dataset",
			Table:       "table",
			TestResults: &pb.BigQueryExport_TestResults{},
		}

		b := &bqExporter{
			maxBatchRowCount: 500,
			maxBatchSize:     6e6,
			putLimiter:       rate.NewLimiter(100, 1),
			batchSem:         semaphore.NewWeighted(100),
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
				tr := m.Struct.(*TestResultRow)
				So(tr.TestID, ShouldBeIn, expectedTestIDs)
				So(tr.ParentInvocation.ID, ShouldBeIn, []string{"a", "b"})
				So(tr.ExportedInvocation.ID, ShouldEqual, "a")
				So(tr.Exonerated, ShouldEqual, tr.TestID == "A" || tr.TestID == "D")
			}
		})

		// To check when encountering an error, the test can run to the end
		// without hanging, or race detector does not detect anything.
		Convey(`fail`, func() {
			err := b.exportTestResultsToBigQuery(ctx, &mockFailInserter{}, "a", bqExport)
			So(err, ShouldErrLike, "some error")
		})
	})
}

type mockTable struct {
	err       error
	callCount int
}

func (t *mockTable) Metadata(ctx context.Context) (*bigquery.TableMetadata, error) {
	t.callCount++
	return nil, t.err
}

func TestBqTableCache(t *testing.T) {
	t.Parallel()
	Convey(`TestCheckBqTableCache`, t, func() {
		ctx := testutil.TestingContext()
		tc := clock.Get(ctx).(testclock.TestClock)
		ctx = caching.WithEmptyProcessCache(ctx)

		Convey(`table not exist`, func() {
			t := &mockTable{
				err: &googleapi.Error{
					Code: http.StatusNotFound,
				},
			}
			bqExport := &pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "not_exist",
			}
			shouldCreateTable, err := checkBqTable(ctx, bqExport, t)
			So(shouldCreateTable, ShouldBeTrue)
			So(err, ShouldBeNil)
		})

		// Random error should not be cached.
		Convey(`random error`, func() {
			t := &mockTable{
				err: fmt.Errorf("random error"),
			}
			bqExport := &pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "random",
			}
			_, err := checkBqTable(ctx, bqExport, t)
			So(err, ShouldErrLike, "random error")
			So(t.callCount, ShouldEqual, 1)

			// Confirms the error is expired in a very short time.
			_, err = checkBqTable(ctx, bqExport, t)
			So(err, ShouldErrLike, "random error")
			So(t.callCount, ShouldEqual, 2)
		})

		Convey(`no error`, func() {
			t := &mockTable{
				err: nil,
			}
			bqExport := &pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "pass",
			}
			_, err := checkBqTable(ctx, bqExport, t)
			So(err, ShouldBeNil)
			So(t.callCount, ShouldEqual, 1)

			// Confirms the cache is working.
			_, err = checkBqTable(ctx, bqExport, t)
			So(err, ShouldBeNil)
			So(t.callCount, ShouldEqual, 1)

			// Confirms the cache is expired as expected.
			tc.Add(6 * time.Minute)
			_, err = checkBqTable(ctx, bqExport, t)
			So(err, ShouldBeNil)
			So(t.callCount, ShouldEqual, 2)
		})
	})
}
