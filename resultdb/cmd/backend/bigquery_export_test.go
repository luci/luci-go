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

package main

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type mockUploader struct {
}

func (u *mockUploader) Put(ctx context.Context, messages ...proto.Message) error {
	time.Sleep(time.Microsecond) // Simulates a time consuming task.
	return nil
}

func TestIsInvocationFinalized(t *testing.T) {
	t.Parallel()
	ctx := testutil.SpannerTestContext(t)
	const token = "update token"
	now := clock.Now(ctx)

	Convey(`not finalized`, t, func() {
		testutil.MustApply(ctx,
			testutil.InsertInvocation("inv_active", pb.Invocation_ACTIVE, token, now, false),
		)

		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		err := isInvocationFinalized(ctx, txn, span.InvocationID("inv_active"))
		So(err, ShouldErrLike, "inv_active is not finalized yet")
	})

	Convey(`finalized`, t, func() {
		testutil.MustApply(ctx,
			testutil.InsertInvocation("inv", pb.Invocation_COMPLETED, token, now, false),
		)

		txn := span.Client(ctx).ReadOnlyTransaction()
		defer txn.Close()

		err := isInvocationFinalized(ctx, txn, span.InvocationID("inv"))
		So(err, ShouldBeNil)
	})
}

func TestExportToBigQuery(t *testing.T) {
	ctx := testutil.SpannerTestContext(t)
	const token = "update token"
	now := clock.Now(ctx)

	Convey(`TestExportTestResultsToBigQuery`, t, func() {
		testutil.MustApply(ctx,
			testutil.InsertInvocation("a", pb.Invocation_COMPLETED, token, now, false))
		testutil.MustApply(ctx, testutil.CombineMutations(
			testutil.InsertTestResults(testutil.MakeTestResults("a", "A", pb.TestStatus_FAIL, pb.TestStatus_PASS)),
			testutil.InsertTestResults(testutil.MakeTestResults("a", "B", pb.TestStatus_CRASH, pb.TestStatus_PASS)),
			testutil.InsertTestResults(testutil.MakeTestResults("a", "C", pb.TestStatus_PASS)),
		)...)

		up := &mockUploader{}
		bqExport := &pb.BigQueryExport{
			Project:     "project",
			Dataset:     "dataset",
			Table:       "table",
			TestResults: &pb.BigQueryExport_TestResults{},
		}

		err := exportTestResultsToBigQuery(ctx, up, span.InvocationID("a"), bqExport, 2)
		So(err, ShouldBeNil)
	})
}
