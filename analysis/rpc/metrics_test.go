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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/perms"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestMetrics(t *testing.T) {
	ftt.Run("With a metrics server", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		request := &pb.ListProjectMetricsRequest{
			Parent: "projects/testproject",
		}

		server := NewMetricsServer()

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
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
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("not a member of luci-analysis-access"))
			assert.Loosely(t, rsp, should.BeNil)
		})
		t.Run("List", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetConfig,
				},
			}

			validateResponse := func(response *pb.ListProjectMetricsResponse) {
				assert.Loosely(t, len(response.Metrics), should.BeGreaterThanOrEqual(1))
				hasDefaultMetric := false
				for _, metric := range response.Metrics {
					assert.Loosely(t, metric.MetricId, should.NotBeEmpty)
					assert.Loosely(t, metric.Name, should.Equal("projects/testproject/metrics/"+metric.MetricId))
					assert.Loosely(t, metric.HumanReadableName, should.NotBeEmpty)
					assert.Loosely(t, metric.Description, should.NotBeEmpty)
					if metric.IsDefault {
						hasDefaultMetric = true
					}
				}
				assert.Loosely(t, hasDefaultMetric, should.BeTrue)
			}

			t.Run("No config access", func(t *ftt.Test) {
				authState.IdentityPermissions = nil
				response, err := server.ListForProject(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.config.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Baseline", func(t *ftt.Test) {
				response, err := server.ListForProject(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				validateResponse(response)

				var clsFailedPresubmitMetric *pb.ProjectMetric
				for _, metric := range response.Metrics {
					if metric.MetricId == metrics.HumanClsFailedPresubmit.ID.String() {
						clsFailedPresubmitMetric = metric
					}
				}
				assert.Loosely(t, clsFailedPresubmitMetric, should.NotBeNil)

				// Verify the overriden values are used for the metric.
				assert.Loosely(t, clsFailedPresubmitMetric.IsDefault, should.BeFalse)
				assert.Loosely(t, clsFailedPresubmitMetric.SortPriority, should.Equal(1000))
			})
			t.Run("No project config", func(t *ftt.Test) {
				configs := make(map[string]*configpb.ProjectConfig)
				err := config.SetTestProjectConfig(ctx, configs)
				assert.Loosely(t, err, should.BeNil)

				response, err := server.ListForProject(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				validateResponse(response)

				var clsFailedPresubmitMetric *pb.ProjectMetric
				for _, metric := range response.Metrics {
					if metric.MetricId == metrics.HumanClsFailedPresubmit.ID.String() {
						clsFailedPresubmitMetric = metric
					}
				}
				assert.Loosely(t, clsFailedPresubmitMetric, should.NotBeNil)

				// Verify default values are used for the metric.
				assert.Loosely(t, clsFailedPresubmitMetric.IsDefault, should.Equal(metrics.HumanClsFailedPresubmit.DefaultConfig.IsDefault))
				assert.Loosely(t, clsFailedPresubmitMetric.SortPriority, should.Equal(metrics.HumanClsFailedPresubmit.DefaultConfig.SortPriority))
			})
		})
	})
}
