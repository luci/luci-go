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
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
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
	req := &sinkpb.ReportTestResultsRequest{
		TestResults: []*sinkpb.TestResult{validTestResult(now)}}
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	recClient := pb.NewMockRecorderClient(ctl)

	Convey("ReportTestResults", t, func() {
		ctx := authtest.MockAuthConfig(context.Background())
		ctx = metadata.NewIncomingContext(
			ctx, metadata.Pairs(AuthTokenKey, authTokenValue("secret")))
		sink, err := newSinkServer(ctx, ServerConfig{
			AuthToken: "secret", Recorder: recClient, Invocation: "inv1"})
		So(err, ShouldBeNil)

		Convey("returns a success for a valid request", func() {
			_, err := sink.ReportTestResults(ctx, req)
			So(err, ShouldBeNil)
			recClient.EXPECT().BatchCreateTestResults(gomock.Any(), gomock.Any())

			// close the server to drain the channel and process the queued items asap.
			ctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			closeSinkServer(ctx, sink)
		})

		Convey("returns an error for a request with an invalid artifact", func() {
			req.TestResults[0].InputArtifacts["input_art2"] = &sinkpb.Artifact{}
			_, err := sink.ReportTestResults(ctx, req)
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
			recClient.EXPECT().BatchCreateTestResults(
				gomock.Any(), gomock.Any()).MaxTimes(0)

			// close the server to drain the channel and process the queued items asap.
			ctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			closeSinkServer(ctx, sink)
		})
	})
}

func TestSinkArtsToRpcArts(t *testing.T) {
	t.Parallel()

	Convey("sinkToRpcArts", t, func() {
		ctx := context.Background()
		sinkArts := map[string]*sinkpb.Artifact{}

		Convey("sets the size of the artifact", func() {
			Convey("with the length of contents", func() {
				sinkArts["art1"] = &sinkpb.Artifact{
					Body: &sinkpb.Artifact_Contents{Contents: []byte("123")}}
				rpcArts := sinkArtsToRpcArts(ctx, sinkArts)
				So(len(rpcArts), ShouldEqual, 1)
				So(rpcArts[0].Size, ShouldEqual, 3)
			})

			Convey("with the size of the file", func() {
				f, err := ioutil.TempFile("", "test-artifact")
				So(err, ShouldBeNil)
				f.Write([]byte("123"))
				f.Close()
				defer os.Remove(f.Name())

				sinkArts["art1"] = &sinkpb.Artifact{
					Body: &sinkpb.Artifact_FilePath{FilePath: f.Name()}}
				rpcArts := sinkArtsToRpcArts(ctx, sinkArts)
				So(len(rpcArts), ShouldEqual, 1)
				So(rpcArts[0].Size, ShouldEqual, 3)
			})

			Convey("with -1 if the file is not accessible", func() {
				sinkArts["art1"] = &sinkpb.Artifact{
					Body: &sinkpb.Artifact_FilePath{FilePath: "does-not-exist/foo/bar"}}
				rpcArts := sinkArtsToRpcArts(ctx, sinkArts)
				So(len(rpcArts), ShouldEqual, 1)
				So(rpcArts[0].Size, ShouldEqual, -1)
			})
		})
	})
}
