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

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/server/auth/authtest"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReportTestResults(t *testing.T) {
	t.Parallel()
	ctl := gomock.NewController(t)
	defer ctl.Finish()

	// basic setup for the server
	ctx := metadata.NewIncomingContext(
		authtest.MockAuthConfig(context.Background()),
		metadata.Pairs(AuthTokenKey, authTokenValue("secret")))
	cfg := testServerConfig(ctl, "", "secret")
	recorder := cfg.Recorder.(*pb.MockRecorderClient)

	Convey("ReportTestResults", t, func() {
		tr, cleanup := validTestResult()
		defer cleanup()
		req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
		cfg.TestIDPrefix = "ninja://foo/bar/"
		cfg.Invocation = "inv1"

		sink, err := newSinkServer(ctx, cfg)
		So(err, ShouldBeNil)
		// This blocks the unit test until the server is fully drained.
		defer func() {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			closeSinkServer(ctx, sink)
		}()

		Convey("works", func() {
			req.TestResults[0].TestId = "TestA"
			_, err := sink.ReportTestResults(ctx, req)
			So(err, ShouldBeNil)

			recorder.EXPECT().BatchCreateTestResults(
				gomock.Any(), matchBatchCreateTestResultsRequest(
					"inv1", &pb.TestResult{
						TestId:      "ninja://foo/bar/TestA",
						ResultId:    tr.ResultId,
						Expected:    tr.Expected,
						Variant:     tr.Variant,
						SummaryHtml: tr.SummaryHtml,
						StartTime:   tr.StartTime,
						Duration:    tr.Duration,
						Tags:        tr.Tags,
					},
				),
			)
		})

		Convey("returns an error if artifacts are invalid", func() {
			req.TestResults[0].Artifacts["art2"] = &sinkpb.Artifact{}
			_, err := sink.ReportTestResults(ctx, req)
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
		})
	})
}
