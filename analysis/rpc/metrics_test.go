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

package rpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/perms"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMetrics(t *testing.T) {
	Convey("With a metrics server", t, func() {
		// For user identification.
		ctx := authtest.MockAuthConfig(context.Background())
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		}
		ctx = auth.WithState(ctx, authState)

		configVersion := time.Date(2025, time.August, 12, 0, 1, 2, 3, time.UTC)
		projectCfg := config.CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_MONORAIL)
		projectCfg.LastUpdated = timestamppb.New(configVersion)
		projectCfg.Metrics = &configpb.Metrics{
			Overrides: []*configpb.Metrics_MetricOverride{
				{
					MetricId:     metrics.HumanClsFailedPresubmit.ID.String(),
					IsDefault:    proto.Bool(false),
					SortPriority: proto.Int32(1000),
				},
			},
		}
		configs := make(map[string]*configpb.ProjectConfig)
		configs["testproject"] = projectCfg

		// Provides datastore implementation needed for project config.
		ctx = memory.Use(ctx)
		err := config.SetTestProjectConfig(ctx, configs)
		So(err, ShouldBeNil)

		request := &pb.ListProjectMetricsRequest{
			Parent: "projects/testproject",
		}

		server := NewMetricsServer()

		Convey("Unauthorised requests are rejected", func() {
			// Ensure no access to luci-analysis-access.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.ListProjectMetricsRequest{
				Parent: "projects/a",
			}

			rsp, err := server.ListForProject(ctx, request)
			So(err, ShouldBeRPCPermissionDenied, "not a member of luci-analysis-access")
			So(rsp, ShouldBeNil)
		})
		Convey("List", func() {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetConfig,
				},
			}

			validateResponse := func(response *pb.ListProjectMetricsResponse) {
				So(len(response.Metrics), ShouldBeGreaterThanOrEqualTo, 1)
				hasDefaultMetric := false
				for _, metric := range response.Metrics {
					So(metric.MetricId, ShouldNotBeEmpty)
					So(metric.Name, ShouldEqual, "projects/testproject/metrics/"+metric.MetricId)
					So(metric.HumanReadableName, ShouldNotBeEmpty)
					So(metric.Description, ShouldNotBeEmpty)
					if metric.IsDefault {
						hasDefaultMetric = true
					}
				}
				So(hasDefaultMetric, ShouldBeTrue)
			}

			Convey("No config access", func() {
				authState.IdentityPermissions = nil
				response, err := server.ListForProject(ctx, request)
				So(err, ShouldBeRPCPermissionDenied, "caller does not have permission analysis.config.get")
				So(response, ShouldBeNil)
			})
			Convey("Baseline", func() {
				response, err := server.ListForProject(ctx, request)
				So(err, ShouldBeNil)
				validateResponse(response)

				var clsFailedPresubmitMetric *pb.ProjectMetric
				for _, metric := range response.Metrics {
					if metric.MetricId == metrics.HumanClsFailedPresubmit.ID.String() {
						clsFailedPresubmitMetric = metric
					}
				}
				So(clsFailedPresubmitMetric, ShouldNotBeNil)

				// Verify the overriden values are used for the metric.
				So(clsFailedPresubmitMetric.IsDefault, ShouldBeFalse)
				So(clsFailedPresubmitMetric.SortPriority, ShouldEqual, 1000)
			})
			Convey("No project config", func() {
				configs := make(map[string]*configpb.ProjectConfig)
				err := config.SetTestProjectConfig(ctx, configs)
				So(err, ShouldBeNil)

				response, err := server.ListForProject(ctx, request)
				So(err, ShouldBeNil)

				validateResponse(response)

				var clsFailedPresubmitMetric *pb.ProjectMetric
				for _, metric := range response.Metrics {
					if metric.MetricId == metrics.HumanClsFailedPresubmit.ID.String() {
						clsFailedPresubmitMetric = metric
					}
				}
				So(clsFailedPresubmitMetric, ShouldNotBeNil)

				// Verify default values are used for the metric.
				So(clsFailedPresubmitMetric.IsDefault, ShouldEqual, metrics.HumanClsFailedPresubmit.DefaultConfig.IsDefault)
				So(clsFailedPresubmitMetric.SortPriority, ShouldEqual, metrics.HumanClsFailedPresubmit.DefaultConfig.SortPriority)
			})
		})
	})
}
