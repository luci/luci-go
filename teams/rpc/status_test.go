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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/teams/internal/teams"
	"go.chromium.org/luci/teams/internal/testutil"
	pb "go.chromium.org/luci/teams/proto/v1"
)

func TestTeams(t *testing.T) {
	ftt.Run("With a Teams server", t, func(t *ftt.Test) {
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
		t.Run("Get", func(t *ftt.Test) {
			t.Run("Anonymous rejected", func(t *ftt.Test) {
				ctx = fakeAuth().anonymous().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/1234567890abcdef1234567890abcdef",
				}
				status, err := server.Get(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("log in"))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Users without app access rejected", func(t *ftt.Test) {
				ctx = fakeAuth().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/1234567890abcdef1234567890abcdef",
				}
				status, err := server.Get(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("not a member of luci-teams-access"))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Read team that exists", func(t *ftt.Test) {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				tb, err := teams.NewTeamBuilder().WithID("1234567890abcdef1234567890abcdef").CreateInDB(ctx)
				assert.Loosely(t, err, should.BeNil)

				request := &pb.GetTeamRequest{
					Name: "teams/1234567890abcdef1234567890abcdef",
				}
				team, err := server.Get(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, team.Name, should.Equal("teams/1234567890abcdef1234567890abcdef"))
				assert.Loosely(t, team.CreateTime.AsTime(), should.Match(tb.CreateTime))
			})
			t.Run("Read own team", func(t *ftt.Test) {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/my",
				}
				_, err := server.Get(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.Unimplemented))
			})
			t.Run("Read of invalid id", func(t *ftt.Test) {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/INVALID",
				}
				_, err := server.Get(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("name: expected format"))
			})
			t.Run("Read of non existing valid id", func(t *ftt.Test) {
				ctx = fakeAuth().withAppAccess().setInContext(ctx)

				request := &pb.GetTeamRequest{
					Name: "teams/abcd1234abcd1234abcd1234abcd1234",
				}
				_, err := server.Get(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("team was not found"))
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
