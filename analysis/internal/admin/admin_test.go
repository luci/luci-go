// Copyright 2024 The LUCI Authors.
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

package admin

import (
	"sort"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"

	pb "go.chromium.org/luci/analysis/internal/admin/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestAdminServer(t *testing.T) {
	ftt.Run("Given an admin server", t, func(t *ftt.Test) {
		ctx, skdr := tq.TestingContext(testutil.TestingContext(), nil)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"service-luci-analysis-admins"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewAdminServer()

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of service-luci-analysis-admins.
				IdentityGroups: []string{"luci-analysis-access"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.BackfillTestResultsRequest{}

			rsp, err := server.BackfillTestResults(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("not a member of service-luci-analysis-admins"))
			assert.Loosely(t, rsp, should.BeNil)
		})
		t.Run("BackfillTestResults", func(t *ftt.Test) {
			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.BackfillTestResultsRequest{
				StartDay: timestamppb.New(time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)),
				EndDay:   timestamppb.New(time.Date(2023, 7, 1, 0, 0, 0, 0, time.UTC)),
			}

			rsp, err := server.BackfillTestResults(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rsp, should.Resemble(&pb.BackfillTestResultsResponse{
				DaysScheduled: 30, // 30 days in June.
			}))

			assert.Loosely(t, skdr.Tasks().Payloads(), should.HaveLength(30))
			var dates []time.Time
			for _, task := range skdr.Tasks().Payloads() {
				tsk, ok := task.(*taskspb.Backfill)
				assert.Loosely(t, ok, should.BeTrue)
				dates = append(dates, tsk.Day.AsTime())
			}
			sort.Slice(dates, func(i, j int) bool {
				return dates[i].Before(dates[j])
			})
			for i, d := range dates {
				assert.Loosely(t, d, should.Resemble(request.StartDay.AsTime().Add(time.Duration(i)*24*time.Hour)))
			}
		})
	})
}
