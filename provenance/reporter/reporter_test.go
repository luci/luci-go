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

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	snooperpb "go.chromium.org/luci/provenance/api/snooperpb/v1"
)

func TestSelfReportClient(t *testing.T) {
	t.Parallel()

	ftt.Run("testing self reports", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		c := &fakeClient{}
		reporter := Report{RClient: c}

		t.Run("cipd admission works", func(t *ftt.Test) {
			ok, err := reporter.ReportCipdAdmission(ctx, "https://chrome-infra-packages-dev.appspot.com/", "package", "deadbeef")
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("git checkout works", func(t *ftt.Test) {
			ok, err := reporter.ReportGitCheckout(ctx, "https://repo.git", "deadbeef", "refs/branch")
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("gcs download works", func(t *ftt.Test) {
			ok, err := reporter.ReportGcsDownload(ctx, "gs://unique/path/to/artifact", "deadbeef")
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("report stage works", func(t *ftt.Test) {
			ok, err := reporter.ReportStage(ctx, snooperpb.TaskStage_FETCH, "", 0)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("report stage fails", func(t *ftt.Test) {
			ok, err := reporter.ReportStage(ctx, snooperpb.TaskStage_STARTED, "", 0)
			assert.Loosely(t, ok, should.Equal(false))
			assert.Loosely(t, err, should.ErrLike("a recipe and pid must be provided when task starts"))
		})
		t.Run("report stage works with client info", func(t *ftt.Test) {
			c.taskStageRequests = nil
			ci := &snooperpb.ClientInfo{
				Kind: &snooperpb.ClientInfo_Swarming_{
					Swarming: &snooperpb.ClientInfo_Swarming{
						TaskId: "1234b56789",
						BotId:  "test.id",
						Server: "https://test-swarm.appspot.com",
					},
				},
			}

			r := Report{RClient: c, ClientInfo: ci}

			ok, err := r.ReportStage(ctx, snooperpb.TaskStage_STARTED, "test-recipe", 1)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(c.taskStageRequests), should.Equal(1))
			assert.Loosely(t, c.taskStageRequests[0].ClientInfo, should.Match(ci))
		})
		t.Run("report stage fails with client info, missing kind", func(t *ftt.Test) {
			c.taskStageRequests = nil
			r := Report{RClient: c, ClientInfo: &snooperpb.ClientInfo{}}
			ok, err := r.ReportStage(ctx, snooperpb.TaskStage_STARTED, "test-recipe", 1)
			assert.Loosely(t, ok, should.Equal(false))
			assert.Loosely(t, err, should.ErrLike("a clientInfo message must contain a kind"))
		})
		t.Run("report cipd digest works", func(t *ftt.Test) {
			ok, err := reporter.ReportCipdDigest(ctx, "deadbeef", "package", "iid")
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("report gcs digest works", func(t *ftt.Test) {
			ok, err := reporter.ReportGcsDigest(ctx, "deadbeef", "gs://bucket/example/1.2.3/app")
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("works", func(t *ftt.Test) {
			ok, err := reporter.ReportPID(ctx, 123)
			assert.Loosely(t, ok, should.Equal(true))
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("fails pid validation", func(t *ftt.Test) {
			ok, err := reporter.ReportPID(ctx, 0)
			assert.Loosely(t, ok, should.Equal(false))
			assert.Loosely(t, err, should.ErrLike("pid must be present"))
		})
	})
}

type fakeClient struct {
	taskStageRequests []*snooperpb.ReportTaskStageRequest
}

func (c *fakeClient) ReportCipd(ctx context.Context, in *snooperpb.ReportCipdRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportGit(ctx context.Context, in *snooperpb.ReportGitRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportGcs(ctx context.Context, in *snooperpb.ReportGcsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportTaskStage(ctx context.Context, in *snooperpb.ReportTaskStageRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	c.taskStageRequests = append(c.taskStageRequests, in)
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportPID(ctx context.Context, in *snooperpb.ReportPIDRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

func (c *fakeClient) ReportArtifactDigest(ctx context.Context, in *snooperpb.ReportArtifactDigestRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
