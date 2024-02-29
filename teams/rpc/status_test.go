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

package rpc

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/teams/internal/teams"
	"go.chromium.org/luci/teams/internal/testutil"
	pb "go.chromium.org/luci/teams/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTeams(t *testing.T) {
	Convey("With a Teams server", t, func() {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-teams-access"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewTeamsServer()
		Convey("Get", func() {
			Convey("Anonymous rejected", func() {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/1234567890abcdef1234567890abcdef",
				}
				status, err := server.Get(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "log in")
				So(status, ShouldBeNil)
			})
			Convey("Users without app access rejected", func() {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/1234567890abcdef1234567890abcdef",
				}
				status, err := server.Get(ctx, request)

				So(err, ShouldBeRPCPermissionDenied, "not a member of luci-teams-access")
				So(status, ShouldBeNil)
			})
			Convey("Read team that exists", func() {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				t, err := teams.NewTeamBuilder().WithID("1234567890abcdef1234567890abcdef").CreateInDB(ctx)
				So(err, ShouldBeNil)

				request := &pb.GetTeamRequest{
					Name: "teams/1234567890abcdef1234567890abcdef",
				}
				team, err := server.Get(ctx, request)

				So(err, ShouldBeNil)
				So(team.Name, ShouldEqual, "teams/1234567890abcdef1234567890abcdef")
				So(team.CreateTime.AsTime(), ShouldEqual, t.CreateTime)
			})
			Convey("Read own team", func() {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/my",
				}
				_, err := server.Get(ctx, request)

				So(err, ShouldHaveRPCCode, codes.Unimplemented)
			})
			Convey("Read of invalid id", func() {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/INVALID",
				}
				_, err := server.Get(ctx, request)

				So(err, ShouldBeRPCInvalidArgument, "name: expected format")
			})
			Convey("Read of non existing valid id", func() {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/abcd1234abcd1234abcd1234abcd1234",
				}
				_, err := server.Get(ctx, request)

				So(err, ShouldBeRPCNotFound, "team was not found")
			})
		})
	})

}

type fakeAuthBuilder struct {
	state *authtest.FakeState
}

func fakeAuth() *fakeAuthBuilder {
	return &fakeAuthBuilder{
		state: &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{},
		},
	}
}
func (a *fakeAuthBuilder) anonymous() *fakeAuthBuilder {
	a.state.Identity = "anonymous:anonymous"
	return a
}
func (a *fakeAuthBuilder) withAppAccess() *fakeAuthBuilder {
	a.state.IdentityGroups = append(a.state.IdentityGroups, teamsAccessGroup)
	return a
}
func (a *fakeAuthBuilder) setInContext(ctx context.Context) context.Context {
	if a.state.Identity == "anonymous:anonymous" && len(a.state.IdentityGroups) > 0 {
		panic("You cannot call any of the with methods on fakeAuthBuilder if you call the anonymous method")
	}
	return auth.WithState(ctx, a.state)
}
