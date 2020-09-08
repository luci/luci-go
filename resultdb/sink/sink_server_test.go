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

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func check(ctx context.Context, cfg ServerConfig, tr *sinkpb.TestResult, expected *pb.TestResult) {
	sink, err := newSinkServer(ctx, cfg)
	sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultIDBase = "foo"
	sink.(*sinkpb.DecoratedSink).Service.(*sinkServer).resultCounter = 100
	So(err, ShouldBeNil)

	req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
	_, err = sink.ReportTestResults(ctx, req)
	So(err, ShouldBeNil)

	cfg.Recorder.(*pb.MockRecorderClient).EXPECT().BatchCreateTestResults(
		gomock.Any(), matchBatchCreateTestResultsRequest(cfg.Invocation, expected))

	// close and drain the server to enforce all the requests processed.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	closeSinkServer(ctx, sink)
}

func TestReportTestResults(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(
		context.Background(),
		metadata.Pairs(AuthTokenKey, authTokenValue("secret")))

	Convey("ReportTestResults", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()

		cfg := testServerConfig(ctl, "", "secret")
		tr, cleanup := validTestResult()
		defer cleanup()

		expected := &pb.TestResult{
			TestId:      tr.TestId,
			ResultId:    tr.ResultId,
			Expected:    tr.Expected,
			SummaryHtml: tr.SummaryHtml,
			StartTime:   tr.StartTime,
			Duration:    tr.Duration,
			Tags:        tr.Tags,
			TestLocation: &pb.TestLocation{
				FileName: tr.TestLocation.FileName,
			},
		}
		Convey("works", func() {
			Convey("with ServerConfig.TestIDPrefix", func() {
				cfg.TestIDPrefix = "ninja://foo/bar/"
				tr.TestId = "HelloWorld.TestA"
				expected.TestId = "ninja://foo/bar/HelloWorld.TestA"
				check(ctx, cfg, tr, expected)
			})

			Convey("with ServerConfig.Variant", func() {
				base := []string{"bucket", "try", "builder", "linux-rel"}
				cfg.BaseVariant = pbutil.Variant(base...)

				expected.Variant = pbutil.Variant(base...)
				check(ctx, cfg, tr, expected)
			})
		})

		Convey("generates a random ResultID, if omitted", func() {
			tr.ResultId = ""
			expected.ResultId = "foo-00101"
			check(ctx, cfg, tr, expected)
		})

		Convey("with ServerConfig.TestLocationBase", func() {
			cfg.TestLocationBase = "//base/"
			tr.TestLocation.FileName = "artifact_dir/a_test.cc"
			expected.TestLocation.FileName = "//base/artifact_dir/a_test.cc"
			check(ctx, cfg, tr, expected)
		})

		Convey("returns an error if artifacts are invalid", func() {
			sink, err := newSinkServer(ctx, cfg)
			So(err, ShouldBeNil)

			tr.Artifacts["art2"] = &sinkpb.Artifact{}
			_, err = sink.ReportTestResults(ctx, &sinkpb.ReportTestResultsRequest{
				TestResults: []*sinkpb.TestResult{tr}})
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			closeSinkServer(ctx, sink)
		})
	})
}
