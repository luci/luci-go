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
	"testing"

	"google.golang.org/protobuf/proto"

	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/tq"

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
			Notification: &resultpb.InvocationReadyForExportNotification{
				RootInvocation:      "invocations/build-123456",
				RootInvocationRealm: "testproject:ci",
				Invocation:          "invocations/build-987654",
				InvocationRealm:     "testproject:test_runner",
				Sources: &resultpb.Sources{
					GitilesCommit: &resultpb.GitilesCommit{
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
