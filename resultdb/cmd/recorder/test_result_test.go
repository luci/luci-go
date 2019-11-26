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

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	//. "go.chromium.org/luci/common/testing/assertions"
)

func TestBatchTestResults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	Convey(`Works`, t, func() {
		now := pbutil.MustTimestampProto(testclock.TestRecentTimeUTC)
		inv := &pb.Invocation{
			Name: "invocations/inv",
			State: pb.Invocation_COMPLETED,
			CreateTime: now,
			FinalizeTime: now,
			Deadline: now,
		}
		trs := []*pb.TestResult{
			{Name: "invocations/inv/tests/foo/results/result_0"},
			{Name: "invocations/inv/tests/foo/results/result_1"},
			{Name: "invocations/inv/tests/foo/results/result_2"},
		}

		Convey(`for one batch`, func() {
			batches := batchTestResults(ctx, inv, trs, 5)
			So(batches, ShouldResemble, []*testResultBatch{
				{
					BatchInv: &pb.Invocation{
						Name: "invocations/inv::batch::0",
						State: pb.Invocation_COMPLETED,
						CreateTime: now,
						FinalizeTime: now,
						Deadline: now,
					},
					TestResults: trs,
				},
			})
		})

		Convey(`for multiple batches`, func() {
			batches := batchTestResults(ctx, inv, trs, 2)
			So(batches, ShouldResemble, []*testResultBatch{
				{
					BatchInv: &pb.Invocation{
						Name: "invocations/inv::batch::0",
						State: pb.Invocation_COMPLETED,
						CreateTime: now,
						FinalizeTime: now,
						Deadline: now,
					},
					TestResults: trs[:2],
				},
				{
					BatchInv: &pb.Invocation{
						Name: "invocations/inv::batch::1",
						State: pb.Invocation_COMPLETED,
						CreateTime: now,
						FinalizeTime: now,
						Deadline: now,
					},
					TestResults: trs[2:],
				},
			})

			Convey(`with batch size a factor of number of TestResults`, func() {
				batches := batchTestResults(ctx, inv, trs, 1)
				So(batches, ShouldResemble, []*testResultBatch{
					{
						BatchInv: &pb.Invocation{
							Name: "invocations/inv::batch::0",
							State: pb.Invocation_COMPLETED,
							CreateTime: now,
							FinalizeTime: now,
							Deadline: now,
						},
						TestResults: trs[:1],
					},
					{
						BatchInv: &pb.Invocation{
							Name: "invocations/inv::batch::1",
							State: pb.Invocation_COMPLETED,
							CreateTime: now,
							FinalizeTime: now,
							Deadline: now,
						},
						TestResults: trs[1:2],
					},
					{
						BatchInv: &pb.Invocation{
							Name: "invocations/inv::batch::2",
							State: pb.Invocation_COMPLETED,
							CreateTime: now,
							FinalizeTime: now,
							Deadline: now,
						},
						TestResults: trs[2:],
					},
				})
			})
		})
	})
}
