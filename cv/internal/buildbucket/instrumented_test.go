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

package buildbucket

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/metrics"

	. "github.com/smartystreets/goconvey/convey"
)

func TestInstrumentedFactory(t *testing.T) {
	t.Parallel()

	Convey("InstrumentedFactory works", t, func() {
		ctx := context.Background()
		if testing.Verbose() {
			ctx = logging.SetLevel(gologger.StdConfig.Use(ctx), logging.Debug)
		}
		ctx = memory.Use(ctx)
		ctx, _, _ = tsmon.WithFakes(ctx)
		tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))
		epoch := datastore.RoundTime(testclock.TestRecentTimeUTC)
		ctx, _ = testclock.UseTime(ctx, epoch)

		const (
			bbHost   = "buildbucket.example.come"
			lProject = "test_proj"
		)
		mockBBClient := &mockBBClient{
			grpcCode: codes.OK,
			latency:  100 * time.Millisecond,
		}
		f := makeInstrumentedFactory(&mockBBClientFactory{
			client: mockBBClient,
		})
		instrumentedClient, err := f.MakeClient(ctx, bbHost, lProject)
		So(err, ShouldBeNil)

		Convey("OK response", func() {
			_, err := instrumentedClient.GetBuild(ctx, &bbpb.GetBuildRequest{
				Id: 123,
			})
			So(err, ShouldBeNil)
			So(tsmonSentCounter(ctx, metrics.Internal.BuildbucketRPCCount, lProject, bbHost, "GetBuild", "OK"), ShouldEqual, 1)
			So(tsmonSentDistr(ctx, metrics.Internal.BuildbucketRPCDurations, lProject, bbHost, "GetBuild", "OK").Sum(), ShouldAlmostEqual, 100)

			Convey("Aware of Batch operation", func() {
				_, err := instrumentedClient.Batch(ctx, &bbpb.BatchRequest{
					Requests: []*bbpb.BatchRequest_Request{
						{
							Request: &bbpb.BatchRequest_Request_GetBuild{
								GetBuild: &bbpb.GetBuildRequest{
									Id: 123,
								},
							},
						},
					},
				})
				So(err, ShouldBeNil)
				So(tsmonSentCounter(ctx, metrics.Internal.BuildbucketRPCCount, lProject, bbHost, "Batch.GetBuild", "OK"), ShouldEqual, 1)
				So(tsmonSentDistr(ctx, metrics.Internal.BuildbucketRPCDurations, lProject, bbHost, "Batch.GetBuild", "OK").Sum(), ShouldAlmostEqual, 100)
			})
		})

		Convey("Error response", func() {
			mockBBClient.grpcCode = codes.NotFound
			mockBBClient.latency = 10 * time.Millisecond
			_, err := instrumentedClient.GetBuild(ctx, &bbpb.GetBuildRequest{
				Id: 123,
			})
			So(err, ShouldNotBeNil)
			So(tsmonSentCounter(ctx, metrics.Internal.BuildbucketRPCCount, lProject, bbHost, "GetBuild", "NOT_FOUND"), ShouldEqual, 1)
			So(tsmonSentDistr(ctx, metrics.Internal.BuildbucketRPCDurations, lProject, bbHost, "GetBuild", "NOT_FOUND").Sum(), ShouldAlmostEqual, 10)

		})
	})
}

func tsmonSentCounter(ctx context.Context, m types.Metric, fieldVals ...any) int64 {
	resetTime := time.Time{}
	v, ok := tsmon.GetState(ctx).Store().Get(ctx, m, resetTime, fieldVals).(int64)
	if !ok {
		panic(fmt.Errorf("either metric isn't a Counter or nothing sent with metric fields %s", fieldVals))
	}
	return v
}

func tsmonSentDistr(ctx context.Context, m types.Metric, fieldVals ...any) *distribution.Distribution {
	resetTime := time.Time{}
	d, ok := tsmon.GetState(ctx).Store().Get(ctx, m, resetTime, fieldVals).(*distribution.Distribution)
	if !ok {
		panic(fmt.Errorf("either metric isn't a Distribution or nothing sent with metric fields %s", fieldVals))
	}
	return d
}

type mockBBClientFactory struct {
	client Client
}

func (m *mockBBClientFactory) MakeClient(ctx context.Context, host, luciProject string) (Client, error) {
	return m.client, nil
}

type mockBBClient struct {
	grpcCode codes.Code
	latency  time.Duration
}

func (m *mockBBClient) GetBuild(ctx context.Context, in *bbpb.GetBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	clock.Get(ctx).(testclock.TestClock).Add(m.latency)
	if m.grpcCode != codes.OK {
		return nil, status.Error(m.grpcCode, "something wrong")
	}
	return &bbpb.Build{}, nil
}
func (m *mockBBClient) SearchBuilds(ctx context.Context, in *bbpb.SearchBuildsRequest, opts ...grpc.CallOption) (*bbpb.SearchBuildsResponse, error) {
	clock.Get(ctx).(testclock.TestClock).Add(m.latency)
	if m.grpcCode != codes.OK {
		return nil, status.Error(m.grpcCode, "something wrong")
	}
	return &bbpb.SearchBuildsResponse{}, nil
}
func (m *mockBBClient) CancelBuild(ctx context.Context, in *bbpb.CancelBuildRequest, opts ...grpc.CallOption) (*bbpb.Build, error) {
	clock.Get(ctx).(testclock.TestClock).Add(m.latency)
	if m.grpcCode != codes.OK {
		return nil, status.Error(m.grpcCode, "something wrong")
	}
	return &bbpb.Build{}, nil

}
func (m *mockBBClient) Batch(ctx context.Context, in *bbpb.BatchRequest, opts ...grpc.CallOption) (*bbpb.BatchResponse, error) {
	clock.Get(ctx).(testclock.TestClock).Add(m.latency)
	if m.grpcCode != codes.OK {
		return nil, status.Error(m.grpcCode, "something wrong")
	}
	return &bbpb.BatchResponse{}, nil
}
