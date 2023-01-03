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

	. "github.com/smartystreets/goconvey/convey"

	pb "go.chromium.org/luci/analysis/proto/v1"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
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
			request := &pb.ListMetricsRequest{}

			rule, err := server.List(ctx, request)
			So(err, ShouldBeRPCPermissionDenied, "not a member of luci-analysis-access")
			So(rule, ShouldBeNil)
		})
		Convey("List", func() {
			request := &pb.ListMetricsRequest{}
			response, err := server.List(ctx, request)
			So(err, ShouldBeNil)
			So(len(response.Metrics), ShouldBeGreaterThanOrEqualTo, 1)
			hasDefaultMetric := false
			for _, metric := range response.Metrics {
				So(metric.MetricId, ShouldNotBeEmpty)
				So(metric.Name, ShouldEqual, "metrics/"+metric.MetricId)
				So(metric.HumanReadableName, ShouldNotBeEmpty)
				So(metric.Description, ShouldNotBeEmpty)
				if metric.IsDefault {
					hasDefaultMetric = true
				}
			}
			So(hasDefaultMetric, ShouldBeTrue)
		})
	})
}
