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
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetCLRunInfo(t *testing.T) {
	t.Parallel()

	Convey("GetCLRunInfo", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		gis := GerritIntegrationServer{}

		gc := &apiv0pb.GerritChange{
			Host:     "g-review.example.com",
			Change:   int64(1),
			Patchset: 39,
		}

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{acls.V0APIAllowGroup},
		})

		Convey("w/o access", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "anonymous:anonymous",
			})
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: gc})
			So(grpcutil.Code(err), ShouldEqual, codes.PermissionDenied)
		})

		Convey("w/ an invalid Gerrit Change", func() {
			invalidGc := &apiv0pb.GerritChange{
				Host:     "bad/host.example.com",
				Change:   int64(1),
				Patchset: 39,
			}
			_, err := gis.GetCLRunInfo(ctx, &apiv0pb.GetCLRunInfoRequest{GerritChange: invalidGc})
			So(grpcutil.Code(err), ShouldEqual, codes.InvalidArgument)
		})

		// TODO(crbug.com/1486976): Create more tests as implementation progresses.
	})
}
