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

	Convey("ReportTestResults", t, func() {
		tr, cleanup := validTestResult()
		req := &sinkpb.ReportTestResultsRequest{TestResults: []*sinkpb.TestResult{tr}}
		defer cleanup()

		// setup a test server
		ctx := metadata.NewIncomingContext(
			authtest.MockAuthConfig(context.Background()),
			metadata.Pairs(AuthTokenKey, authTokenValue("secret")))
		cfg := testServerConfig(ctl, "", "secret")
		cfg.Invocation = "inv1"
		sink, err := newSinkServer(ctx, cfg)
		So(err, ShouldBeNil)

		defer func() {
			// close the server to drain the channel and process the queued items.
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			closeSinkServer(ctx, sink)
		}()

		// mock
		recorder := cfg.Recorder.(*pb.MockRecorderClient)

		Convey("creates TestResult", func() {
			_, err := sink.ReportTestResults(ctx, req)
			So(err, ShouldBeNil)

			// rdb_channel should invoke recorder.BatchCreateTestResults()
			recorder.EXPECT().BatchCreateTestResults(gomock.Any(), invEq(cfg.Invocation))
		})

		Convey("returns an error if artifacts are invalid", func() {
			req.TestResults[0].Artifacts["art2"] = &sinkpb.Artifact{}
			_, err := sink.ReportTestResults(ctx, req)
			So(status.Code(err), ShouldEqual, codes.InvalidArgument)
		})
	})
}
