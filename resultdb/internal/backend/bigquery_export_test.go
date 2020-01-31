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
	"sync"
	"testing"

	"go.chromium.org/luci/common/bq"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	bqpb "go.chromium.org/luci/resultdb/proto/bq/v1"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

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
			testutil.InsertInvocation("a", pb.Invocation_FINALIZED, nil))
		testutil.MustApply(ctx, testutil.CombineMutations(
			// Test results and exonerations have the same variants.
			testutil.InsertTestResults(testutil.MakeTestResults("a", "A", pb.TestStatus_FAIL, pb.TestStatus_PASS)),
			testutil.InsertTestExonerations("a", "A", pbutil.Variant("k1", "v1", "k2", "v2"), 1),
			// Test results and exonerations have different variants.
			testutil.InsertTestResults(testutil.MakeTestResults("a", "B", pb.TestStatus_CRASH, pb.TestStatus_PASS)),
			testutil.InsertTestExonerations("a", "B", pbutil.Variant("k1", "v1"), 1),
			// Passing test result without exoneration.
			testutil.InsertTestResults(testutil.MakeTestResults("a", "C", pb.TestStatus_PASS)),
		)...)

		bqExport := &pb.BigQueryExport{
			Project:     "project",
			Dataset:     "dataset",
			Table:       "table",
			TestResults: &pb.BigQueryExport_TestResults{},
		}

		Convey(`success`, func() {
			i := &mockPassInserter{}
			err := exportTestResultsToBigQuery(ctx, i, "a", bqExport, 2)
			So(err, ShouldBeNil)

			i.mu.Lock()
			defer i.mu.Unlock()
			So(len(i.insertedMessages), ShouldEqual, 5)

			expectedTestIDs := []string{"A", "B", "C"}
			for _, m := range i.insertedMessages {
				invID, testID, _ := span.MustParseTestResultName(m.InsertID)
				So(invID, ShouldEqual, "a")
				So(testID, ShouldBeIn, expectedTestIDs)
				tr := m.Message.(*bqpb.TestResultRow)
				So(tr.Exoneration.Exonerated, ShouldEqual, testID == "A")
			}
		})

		// To check when encountering an error, the test can run to the end
		// without hanging, or race detector does not detect anything.
		Convey(`fail`, func() {
			err := exportTestResultsToBigQuery(ctx, &mockFailInserter{}, "a", bqExport, 2)
			So(err, ShouldErrLike, "some error")
		})
	})
}
