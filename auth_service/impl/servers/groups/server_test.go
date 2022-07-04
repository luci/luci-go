// Copyright 2021 The LUCI Authors.
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
package groups

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGroupsServer(t *testing.T) {
	t.Parallel()
	srv := Server{}
	createdTime := time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC)

	Convey("ListGroups RPC call", t, func() {
		ctx := memory.Use(context.Background())

		// Groups built from model.AuthGroup definition.
		So(datastore.Put(ctx,
			&model.AuthGroup{
				ID:     "z-test-group",
				Parent: model.RootKey(ctx),
				Members: []string{
					"user:test-user-1",
					"user:test-user-2",
				},
				Globs: []string{
					"test-user-1@example.com",
					"test-user-2@example.com",
				},
				Nested: []string{
					"group/tester",
				},
				Description: "This is a test group.",
				Owners:      "testers",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1@example.com",
			},
			&model.AuthGroup{
				ID:     "test-group-2",
				Parent: model.RootKey(ctx),
				Members: []string{
					"user:test-user-2",
				},
				Globs: []string{
					"test-user-2@example.com",
				},
				Nested: []string{
					"group/test-group",
				},
				Description: "This is another test group.",
				Owners:      "test-group",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-2@example.com",
			},
			&model.AuthGroup{
				ID:      "test-group-3",
				Parent:  model.RootKey(ctx),
				Members: []string{},
				Globs:   []string{},
				Nested: []string{
					"group/tester",
				},
				Description: "This is yet another test group.",
				Owners:      "testers",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1@example.com",
			}), ShouldBeNil)

		// What expected response should be, built with pb.
		expectedResp := &rpcpb.ListGroupsResponse{
			Groups: []*rpcpb.AuthGroup{
				{
					Name:        "test-group-2",
					Description: "This is another test group.",
					Owners:      "test-group",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-2@example.com",
				},
				{
					Name:        "test-group-3",
					Description: "This is yet another test group.",
					Owners:      "testers",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-1@example.com",
				},
				{
					Name:        "z-test-group",
					Description: "This is a test group.",
					Owners:      "testers",
					CreatedTs:   timestamppb.New(createdTime),
					CreatedBy:   "user:test-user-1@example.com",
				},
			},
		}

		resp, err := srv.ListGroups(ctx, &emptypb.Empty{})
		So(err, ShouldBeNil)
		So(resp.Groups, ShouldResembleProto, expectedResp.Groups)
	})

	Convey("GetGroup RPC call", t, func() {
		ctx := memory.Use(context.Background())

		request := &rpcpb.GetGroupRequest{
			Name: "test-group",
		}

		_, err := srv.GetGroup(ctx, request)
		So(err, ShouldHaveGRPCStatus, codes.NotFound)

		// Groups built from model.AuthGroup definition.
		So(datastore.Put(ctx,
			&model.AuthGroup{
				ID:     "test-group",
				Parent: model.RootKey(ctx),
				Members: []string{
					"user:test-user-1",
					"user:test-user-2",
				},
				Globs: []string{
					"test-user-1@example.com",
					"test-user-2@example.com",
				},
				Nested: []string{
					"group/tester",
				},
				Description: "This is a test group.",
				Owners:      "testers",
				CreatedTS:   createdTime,
				CreatedBy:   "user:test-user-1@example.com",
			}), ShouldBeNil)

		expectedResponse := &rpcpb.AuthGroup{
			Name: "test-group",
			Members: []string{
				"user:test-user-1",
				"user:test-user-2",
			},
			Globs: []string{
				"test-user-1@example.com",
				"test-user-2@example.com",
			},
			Nested: []string{
				"group/tester",
			},
			Description: "This is a test group.",
			Owners:      "testers",
			CreatedTs:   timestamppb.New(createdTime),
			CreatedBy:   "user:test-user-1@example.com",
		}

		actualGroupResponse, err := srv.GetGroup(ctx, request)
		So(err, ShouldBeNil)
		So(actualGroupResponse, ShouldResembleProto, expectedResponse)

	})

	Convey("CreateGroup RPC call", t, func() {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, _ = tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		Convey("Invalid name", func() {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "#^&",
					Description: "This is a group with an invalid name",
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		})

		Convey("Invalid members", func() {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a group with invalid members.",
					Owners:      "test-group",
					Members:     []string{"no-prefix@identity.com"},
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		})

		Convey("Invalid globs", func() {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a group with invalid members.",
					Owners:      "test-group",
					Globs:       []string{"*@no-prefix.com"},
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
		})

		Convey("Group already exists", func() {
			So(datastore.Put(ctx,
				&model.AuthGroup{
					ID:          "test-group",
					Parent:      model.RootKey(ctx),
					Description: "This is a test group.",
					Owners:      "testers",
					CreatedTS:   createdTime,
					CreatedBy:   "user:test-user-1@example.com",
				}), ShouldBeNil)

			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a group that already exists",
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			So(err, ShouldHaveGRPCStatus, codes.AlreadyExists)
		})

		Convey("Group refers to another group that doesn't exist", func() {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a test group.",
					Owners:      "invalid-owner",
					Nested:      []string{"bad1, bad2"},
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "some referenced groups don't exist: bad1, bad2, invalid-owner")
		})

		Convey("Successful creation", func() {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a test group.",
					Owners:      "test-group",
				},
			}

			resp, err := srv.CreateGroup(ctx, request)
			So(err, ShouldBeNil)
			So(resp.Name, ShouldEqual, "test-group")
			So(resp.Description, ShouldEqual, "This is a test group.")
			So(resp.Owners, ShouldEqual, "test-group")
			So(resp.CreatedBy, ShouldEqual, "user:someone@example.com")
			So(resp.CreatedTs.Seconds, ShouldNotBeZeroValue)
		})
	})

	Convey("DeleteGroup RPC call", t, func() {
		ctx := memory.Use(context.Background())
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, _ = tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		Convey("Group not found", func() {
			request := &rpcpb.DeleteGroupRequest{
				Name: "non-existent-group",
			}

			_, err := srv.DeleteGroup(ctx, request)
			So(err, ShouldHaveGRPCStatus, codes.NotFound)
		})

		Convey("Permissions", func() {
			So(datastore.Put(ctx,
				&model.AuthGroup{
					ID:     "test-group",
					Parent: model.RootKey(ctx),
					Members: []string{
						"user:test-user-1",
						"user:test-user-2",
					},
					Globs: []string{
						"test-user-1@example.com",
						"test-user-2@example.com",
					},
					Nested: []string{
						"group/tester",
					},
					Description: "This is a test group.",
					Owners:      "owners",
					CreatedTS:   createdTime,
					CreatedBy:   "user:test-user-1@example.com",
				}), ShouldBeNil)

			Convey("Anonymous is denied", func() {
				ctx := auth.WithState(ctx, &authtest.FakeState{})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			})

			Convey("Normal user is denied", func() {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			})

			Convey("Group owner succeeds", func() {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"owners"},
				})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				So(err, ShouldBeNil)
			})

			Convey("Admin succeeds", func() {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{model.AdminGroup},
				})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				So(err, ShouldBeNil)
			})
		})
	})

	Convey("GetSubgraph RPC call", t, func() {
		const (
			// Identities, groups, globs
			owningGroup = "owning-group"
			nestedGroup = "nested-group"
			soloGroup   = "solo-group"
			testUser0   = "user:m0@example.com"
			testUser1   = "user:m1@example.com"
			testUser2   = "user:t2@example.com"
			testGlob0   = "user:m*@example.com"
			testGlob1   = "user:t2*"
		)

		ctx := memory.Use(context.Background())
		So(datastore.Put(ctx,
			&model.AuthGroup{
				ID:     owningGroup,
				Parent: model.RootKey(ctx),
				Members: []string{
					testUser0,
					testUser1,
				},
				Nested: []string{
					nestedGroup,
				},
			},
			&model.AuthGroup{
				ID:     soloGroup,
				Parent: model.RootKey(ctx),
				Globs: []string{
					testGlob1,
				},
			},
			&model.AuthGroup{
				ID:     nestedGroup,
				Parent: model.RootKey(ctx),
				Members: []string{
					testUser2,
				},
				Globs: []string{
					testGlob0,
					testGlob1,
				},
				Owners: owningGroup,
			}), ShouldBeNil)

		Convey("Identity principal", func() {
			request := &rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_IDENTITY,
					Name: testUser0,
				},
			}

			actualSubgraph, err := srv.GetSubgraph(ctx, request)
			So(err, ShouldBeNil)

			expectedSubgraph := &rpcpb.Subgraph{
				Nodes: []*rpcpb.Node{
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_IDENTITY,
							Name: testUser0,
						},
						IncludedBy: []int32{1, 3},
					},
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_GLOB,
							Name: testGlob0,
						},
						IncludedBy: []int32{2},
					},
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_GROUP,
							Name: nestedGroup,
						},
						IncludedBy: []int32{3},
					},
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_GROUP,
							Name: owningGroup,
						},
					},
				},
			}

			So(actualSubgraph, ShouldResemble, expectedSubgraph)
		})

		Convey("Group principal", func() {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: nestedGroup,
				},
			}

			actualSubgraph, err := srv.GetSubgraph(ctx, &request)
			So(err, ShouldBeNil)

			expectedSubgraph := &rpcpb.Subgraph{
				Nodes: []*rpcpb.Node{
					{
						Principal:  request.Principal,
						IncludedBy: []int32{1},
					},
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_GROUP,
							Name: owningGroup,
						},
					},
				},
			}

			So(actualSubgraph, ShouldResemble, expectedSubgraph)

		})

		Convey("Glob principal", func() {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GLOB,
					Name: testGlob1,
				},
			}

			actualSubgraph, err := srv.GetSubgraph(ctx, &request)
			So(err, ShouldBeNil)

			expectedSubgraph := &rpcpb.Subgraph{
				Nodes: []*rpcpb.Node{
					{
						Principal:  request.Principal,
						IncludedBy: []int32{1, 3},
					},
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_GROUP,
							Name: nestedGroup,
						},
						IncludedBy: []int32{2},
					},
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_GROUP,
							Name: owningGroup,
						},
					},
					{
						Principal: &rpcpb.Principal{
							Kind: rpcpb.PrincipalKind_GROUP,
							Name: soloGroup,
						},
					},
				},
			}

			So(actualSubgraph, ShouldResemble, expectedSubgraph)
		})

		Convey("Unspecified Principal kind", func() {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_PRINCIPAL_KIND_UNSPECIFIED,
					Name: "aeua//",
				},
			}

			_, err := srv.GetSubgraph(ctx, &request)
			So(err.Error(), ShouldContainSubstring, "invalid principal kind")

		})

		Convey("Group principal not in groups graph", func() {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: "i-dont-exist",
				},
			}

			_, err := srv.GetSubgraph(ctx, &request)
			So(err.Error(), ShouldContainSubstring, "no such group")
		})
	})
}
