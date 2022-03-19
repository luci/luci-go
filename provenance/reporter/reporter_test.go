// Copyright 2022 The LUCI Authors.
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

package reporter

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	snooperpb "go.chromium.org/luci/provenance/api/snooperpb/v1"
)

func TestReportCipdAdmission(t *testing.T) {
	t.Parallel()

	Convey("testing self reports", t, func() {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		reporter := Report{sClient: &fakeClient{}}

		Convey("cipd admission works", func() {
			So(reporter.ReportCipdAdmission(ctx, "package", "deadbeef"), ShouldBeNil)
		})
		Convey("git checkout works", func() {
			So(reporter.ReportGitCheckout(ctx, "https://repo.git", "deadbeef", "refs/branch"), ShouldBeNil)
		})
		Convey("report stage works", func() {
			So(reporter.ReportStage(ctx, snooperpb.TaskStage_FETCH, ""), ShouldBeNil)
		})
		Convey("report stage fails", func() {
			So(reporter.ReportStage(ctx, snooperpb.TaskStage_STARTED, ""), ShouldErrLike, "need to report recipe when task starts")
		})
		Convey("report cipd digest works", func() {
			So(reporter.ReportCipdDigest(ctx, "deadbeef", "package", "iid"), ShouldBeNil)
		})
		Convey("report gcs digest works", func() {
			So(reporter.ReportGcsDigest(ctx, "deadbeef", "gs://bucket/example/1.2.3/app"), ShouldBeNil)
		})
	})
}

type fakeClient struct{}

func (c *fakeClient) ReportCipd(ctx context.Context, in *snooperpb.ReportCipdRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportGit(ctx context.Context, in *snooperpb.ReportGitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportTaskStage(ctx context.Context, in *snooperpb.ReportTaskStageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportArtifactDigest(ctx context.Context, in *snooperpb.ReportArtifactDigestRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
