// Copyright 2020 The LUCI Authors.
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
package sink

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/resultdb/pbutil"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
)

// validTestResult returns a valid sinkpb.TestResult sample.
func validTestResult(now time.Time) *sinkpb.TestResult {
	st, _ := ptypes.TimestampProto(now.Add(-2 * time.Minute))
	return &sinkpb.TestResult{
		TestId:      "this is testID",
		ResultId:    "result_id1",
		Variant:     pbutil.Variant("a", "b"),
		Expected:    true,
		Status:      sinkpb.TestStatus_PASS,
		SummaryHtml: "HTML summary",
		StartTime:   st,
		Duration:    ptypes.DurationProto(time.Minute),
		Tags:        pbutil.StringPairs("k1", "v1"),
		InputArtifacts: map[string]*sinkpb.Artifact{
			"input_art1": &sinkpb.Artifact{
				Body:        &sinkpb.Artifact_FilePath{"/tmp/foo"},
				ContentType: "text/plain",
			},
		},
		OutputArtifacts: map[string]*sinkpb.Artifact{
			"output_art1": &sinkpb.Artifact{
				Body:        &sinkpb.Artifact_Contents{[]byte("contents")},
				ContentType: "text/plain",
			},
		},
	}
}

func TestReportTestResults(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC

	Convey("ReportTestResults", t, func() {
		sink := &sinkServer{}
		ctx := context.Background()
		req := &sinkpb.ReportTestResultsRequest{
			TestResults: []*sinkpb.TestResult{validTestResult(now)},
		}

		Convey("returns a success for a valid request", func() {
			_, err := sink.ReportTestResults(ctx, req)
			So(err, ShouldBeNil)
		})

		Convey("returns an error for a request with an invalid artifact", func() {
			req.TestResults[0].InputArtifacts["input_art2"] = &sinkpb.Artifact{}
			_, err := sink.ReportTestResults(ctx, req)
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
		})
	})
}
