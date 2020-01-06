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
	"fmt"
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

type mockPassUploader struct {
}

func (u *mockPassUploader) Put(ctx context.Context, messages ...proto.Message) error {
	time.Sleep(time.Millisecond) // Simulates a time consuming task.
	return nil
}

type mockFailUploader struct {
}

func (u *mockFailUploader) Put(ctx context.Context, messages ...proto.Message) error {
	time.Sleep(time.Millisecond) // Simulates a time consuming task.
	return fmt.Errorf("some error")
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
			testutil.InsertTestResults(testutil.MakeTestResults("a", "C", pb.TestStatus_PASS)),
		)...)

		bqExport := &pb.BigQueryExport{
			Project:     "project",
			Dataset:     "dataset",
			Table:       "table",
			TestResults: &pb.BigQueryExport_TestResults{},
		}

		err := exportTestResultsToBigQuery(ctx, &mockPassUploader{}, span.InvocationID("a"), bqExport, 2)
		So(err, ShouldBeNil)

		err = exportTestResultsToBigQuery(ctx, &mockFailUploader{}, span.InvocationID("a"), bqExport, 2)
		So(err, ShouldErrLike, "some error")
	})
}
