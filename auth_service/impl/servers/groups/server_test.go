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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	fieldmaskpb "google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/impl/model"
)

func TestGroupsServer(t *testing.T) {
	t.Parallel()

	createdTime := time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC)
	modifiedTime := time.Date(2022, time.July, 4, 15, 45, 0, 0, time.UTC)
	versionedEntity := model.AuthVersionedEntityMixin{
		ModifiedBy: "user:test-modifier@example.com",
		ModifiedTS: modifiedTime,
	}
	// Etag derived from the above modified time.
	etag := `W/"MjAyMi0wNy0wNFQxNTo0NTowMFo="`
	// Last-Modified header derived from the above modified time.
	lastModifiedHeader := "Mon, 04 Jul 2022 15:45:00 +0000"

	ftt.Run("ListGroups RPC call", t, func(t *ftt.Test) {
		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"testers"},
		})
		testTime := time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)
		ctx, tc := testclock.UseTime(ctx, testTime)

		// Groups built from model.AuthGroup definition.
		assert.Loosely(t, datastore.Put(ctx,
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
				Description:              "This is a test group.",
				Owners:                   "testers",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-1@example.com",
				AuthVersionedEntityMixin: versionedEntity,
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
				Description:              "This is another test group.",
				Owners:                   "test-group",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-2@example.com",
				AuthVersionedEntityMixin: versionedEntity,
			},
			&model.AuthGroup{
				ID:      "test-group-3",
				Parent:  model.RootKey(ctx),
				Members: []string{},
				Globs:   []string{},
				Nested: []string{
					"group/tester",
				},
				Description:              "This is yet another test group.",
				Owners:                   "testers",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-1@example.com",
				AuthVersionedEntityMixin: versionedEntity,
			},
			&model.AuthGroup{
				ID:     "google/test-google-group",
				Parent: model.RootKey(ctx),
				Members: []string{
					"user:test-user-1",
					"user:test-user-2",
				},
				Description:              "This is a test google group.",
				Owners:                   "test-group",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-2@example.com",
				AuthVersionedEntityMixin: versionedEntity,
			},
		), should.BeNil)
		assert.Loosely(t, putRev(ctx, 123), should.BeNil)

		// What expected response should be, built with pb.
		expectedResp := &rpcpb.ListGroupsResponse{
			Groups: []*rpcpb.AuthGroup{
				{
					Name:                 "google/test-google-group",
					Description:          "This is a test google group.",
					Owners:               "test-group",
					CreatedTs:            timestamppb.New(createdTime),
					CreatedBy:            "user:test-user-2@example.com",
					CallerCanModify:      false,
					CallerCanViewMembers: false,
					Etag:                 etag,
				},
				{
					Name:                 "test-group-2",
					Description:          "This is another test group.",
					Owners:               "test-group",
					CreatedTs:            timestamppb.New(createdTime),
					CreatedBy:            "user:test-user-2@example.com",
					CallerCanModify:      false,
					CallerCanViewMembers: true,
					Etag:                 etag,
				},
				{
					Name:                 "test-group-3",
					Description:          "This is yet another test group.",
					Owners:               "testers",
					CreatedTs:            timestamppb.New(createdTime),
					CreatedBy:            "user:test-user-1@example.com",
					CallerCanModify:      true,
					CallerCanViewMembers: true,
					Etag:                 etag,
				},
				{
					Name:                 "z-test-group",
					Description:          "This is a test group.",
					Owners:               "testers",
					CreatedTs:            timestamppb.New(createdTime),
					CreatedBy:            "user:test-user-1@example.com",
					CallerCanModify:      true,
					CallerCanViewMembers: true,
					Etag:                 etag,
				},
			},
		}

		resp, err := srv.ListGroups(ctx, &emptypb.Empty{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp.Groups, should.Match(expectedResp.Groups))

		t.Run("returns cached groups", func(t *ftt.Test) {
			tc.Add(maxStaleness - 1)
			resp, err := srv.ListGroups(ctx, &emptypb.Empty{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Groups, should.Match(expectedResp.Groups))
		})
	})

	ftt.Run("GetGroup RPC call", t, func(t *ftt.Test) {
		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"testers"},
		})
		request := &rpcpb.GetGroupRequest{}

		_, err := srv.GetGroup(ctx, request)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))

		request.Name = "test-group"
		_, err = srv.GetGroup(ctx, request)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))

		// Groups built from model.AuthGroup definition.
		assert.Loosely(t, datastore.Put(ctx,
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
				Description:              "This is a test group.",
				Owners:                   "testers",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-1@example.com",
				AuthVersionedEntityMixin: versionedEntity,
			}), should.BeNil)

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
			Description:          "This is a test group.",
			Owners:               "testers",
			CreatedTs:            timestamppb.New(createdTime),
			CreatedBy:            "user:test-user-1@example.com",
			CallerCanModify:      true,
			CallerCanViewMembers: true,
			Etag:                 etag,
		}

		actualGroupResponse, err := srv.GetGroup(ctx, request)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualGroupResponse, should.Match(expectedResponse))
		t.Run("AdminGroup can view external group", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{model.AdminGroup},
			})
			request := &rpcpb.GetGroupRequest{
				Name: "google/test-group",
			}
			_, err := srv.GetGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, datastore.Put(ctx,
				&model.AuthGroup{
					ID:     "google/test-group",
					Parent: model.RootKey(ctx),
					Members: []string{
						"user:test-user-1",
						"user:test-user-2",
					},
					Description:              "This is a test google group.",
					Owners:                   "testers",
					CreatedTS:                createdTime,
					CreatedBy:                "user:test-user-1@example.com",
					AuthVersionedEntityMixin: versionedEntity,
				}), should.BeNil)

			expectedResponse := &rpcpb.AuthGroup{
				Name: "google/test-group",
				Members: []string{
					"user:test-user-1",
					"user:test-user-2",
				},
				Description:          "This is a test google group.",
				Owners:               "testers",
				CreatedTs:            timestamppb.New(createdTime),
				CreatedBy:            "user:test-user-1@example.com",
				CallerCanModify:      false,
				CallerCanViewMembers: true,
				Etag:                 etag,
			}
			actualGroupResponse, err := srv.GetGroup(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualGroupResponse, should.Match(expectedResponse))
		})
		t.Run("Non admin group redacts external group", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{"testers"},
			})
			request := &rpcpb.GetGroupRequest{
				Name: "google/test-group",
			}
			_, err := srv.GetGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, datastore.Put(ctx,
				&model.AuthGroup{
					ID:     "google/test-group",
					Parent: model.RootKey(ctx),
					Members: []string{
						"user:test-user-1",
						"user:test-user-2",
					},
					Description:              "This is a test google group.",
					Owners:                   "testers",
					CreatedTS:                createdTime,
					CreatedBy:                "user:test-user-1@example.com",
					AuthVersionedEntityMixin: versionedEntity,
				}), should.BeNil)

			expectedResponse := &rpcpb.AuthGroup{
				Name:                 "google/test-group",
				Description:          "This is a test google group.",
				Owners:               "testers",
				CreatedTs:            timestamppb.New(createdTime),
				CreatedBy:            "user:test-user-1@example.com",
				CallerCanModify:      false,
				CallerCanViewMembers: false,
				NumRedacted:          2,
				Etag:                 etag,
			}
			actualGroupResponse, err := srv.GetGroup(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualGroupResponse, should.Match(expectedResponse))
		})

	})

	ftt.Run("GetLegacyAuthGroup REST call", t, func(t *ftt.Test) {
		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"testers"},
		})

		t.Run("empty name", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "groupName", Value: ""},
				},
				Writer: rw,
			}
			err := srv.GetLegacyAuthGroup(rctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
		})

		t.Run("not found", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "groupName", Value: "test-group"},
				},
				Writer: rw,
			}
			err := srv.GetLegacyAuthGroup(rctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
		})

		t.Run("returns group details", func(t *ftt.Test) {
			// Setup by adding a group in Datastore.
			assert.Loosely(t, datastore.Put(ctx,
				&model.AuthGroup{
					ID:     "test-group",
					Parent: model.RootKey(ctx),
					Members: []string{
						"user:test-user-1@example.com",
						"user:test-user-2@example.com",
					},
					Globs: []string{
						"*@example.com",
					},
					Nested: []string{
						"group/tester",
					},
					Description:              "This is a test group.",
					Owners:                   "testers",
					CreatedTS:                createdTime,
					CreatedBy:                "user:test-user-1@example.com",
					AuthVersionedEntityMixin: versionedEntity,
				}), should.BeNil)

			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "groupName", Value: "test-group"},
				},
				Writer: rw,
			}
			err := srv.GetLegacyAuthGroup(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]AuthGroupJSON{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]AuthGroupJSON{
				"group": {
					Name:        "test-group",
					Description: "This is a test group.",
					Owners:      "testers",
					Members: []string{
						"user:test-user-1@example.com",
						"user:test-user-2@example.com",
					},
					Globs:           []string{"*@example.com"},
					Nested:          []string{"group/tester"},
					CreatedBy:       "user:test-user-1@example.com",
					CreatedTS:       createdTime.UnixMicro(),
					ModifiedBy:      "user:test-modifier@example.com",
					ModifiedTS:      modifiedTime.UnixMicro(),
					CallerCanModify: true,
				},
			}))
			assert.Loosely(t, rw.Header().Get("Last-Modified"), should.Equal(lastModifiedHeader))
		})
	})

	ftt.Run("CreateGroup RPC call", t, func(t *ftt.Test) {
		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, _ = tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		t.Run("Invalid name", func(t *ftt.Test) {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "#^&",
					Description: "This is a group with an invalid name",
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Invalid name (looks like external group)", func(t *ftt.Test) {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "mdb/foo",
					Description: "This is a group with an invalid name",
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Invalid members", func(t *ftt.Test) {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a group with invalid members.",
					Owners:      "test-group",
					Members:     []string{"no-prefix@identity.com"},
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Invalid globs", func(t *ftt.Test) {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a group with invalid members.",
					Owners:      "test-group",
					Globs:       []string{"*@no-prefix.com"},
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Group already exists", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx,
				&model.AuthGroup{
					ID:          "test-group",
					Parent:      model.RootKey(ctx),
					Description: "This is a test group.",
					Owners:      "testers",
					CreatedTS:   createdTime,
					CreatedBy:   "user:test-user-1@example.com",
				}), should.BeNil)

			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a group that already exists",
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.AlreadyExists))
		})

		t.Run("Group refers to another group that doesn't exist", func(t *ftt.Test) {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a test group.",
					Owners:      "invalid-owner",
					Nested:      []string{"bad1, bad2"},
				},
			}
			_, err := srv.CreateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("some referenced groups don't exist: bad1, bad2, invalid-owner"))
		})

		t.Run("Successful creation", func(t *ftt.Test) {
			request := &rpcpb.CreateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "This is a test group.",
					Owners:      "test-group",
				},
			}

			resp, err := srv.CreateGroup(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.Name, should.Equal("test-group"))
			assert.Loosely(t, resp.Description, should.Equal("This is a test group."))
			assert.Loosely(t, resp.Owners, should.Equal("test-group"))
			assert.Loosely(t, resp.CreatedBy, should.Equal("user:someone@example.com"))
			assert.Loosely(t, resp.CreatedTs.Seconds, should.NotBeZero)
		})
	})

	ftt.Run("UpdateGroup RPC call", t, func(t *ftt.Test) {
		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"owners"},
		})
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, _ = tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		assert.Loosely(t, datastore.Put(ctx,
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
				Description:              "This is a test group.",
				Owners:                   "owners",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-1@example.com",
				AuthVersionedEntityMixin: versionedEntity,
			}), should.BeNil)

		t.Run("Cannot update external group", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "mdb/foo",
					Description: "update",
				},
			}

			_, err := srv.UpdateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("cannot update external group"))
		})

		t.Run("Group not found", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "non-existent-group",
					Description: "update",
				},
			}

			_, err := srv.UpdateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		})

		t.Run("Invalid field mask", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "update",
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"bad"}},
			}

			_, err := srv.UpdateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Set owners to group that doesn't exist", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:   "test-group",
					Owners: "non-existent",
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"owners"}},
			}

			_, err := srv.UpdateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Set invalid member identity", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:    "test-group",
					Members: []string{"bad"},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"members"}},
			}

			_, err := srv.UpdateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("Cyclic dependency", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:   "test-group",
					Nested: []string{"test-group"},
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"nested"}},
			}

			_, err := srv.UpdateGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
		})

		t.Run("Permissions", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "update",
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}},
			}
			t.Run("Anonymous is denied", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{})
				_, err := srv.UpdateGroup(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Normal user is denied", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				_, err := srv.UpdateGroup(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Group owner succeeds", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"owners"},
				})
				_, err := srv.UpdateGroup(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("Admin succeeds", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{model.AdminGroup},
				})
				_, err := srv.UpdateGroup(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("Etags", func(t *ftt.Test) {
			request := &rpcpb.UpdateGroupRequest{
				Group: &rpcpb.AuthGroup{
					Name:        "test-group",
					Description: "update",
				},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}},
			}

			t.Run("Incorrect etag is aborted", func(t *ftt.Test) {
				request.Group.Etag = "blah"
				_, err := srv.UpdateGroup(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Aborted))
			})

			t.Run("Empty etag succeeds", func(t *ftt.Test) {
				request.Group.Etag = ""
				_, err := srv.UpdateGroup(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("Correct etag succeeds if present", func(t *ftt.Test) {
				request.Group.Etag = etag
				_, err := srv.UpdateGroup(ctx, request)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})

	ftt.Run("DeleteGroup RPC call", t, func(t *ftt.Test) {
		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := memory.Use(context.Background())
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, _ = tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		assert.Loosely(t, datastore.Put(ctx,
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
				Description:              "This is a test group.",
				Owners:                   "owners",
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-user-1@example.com",
				AuthVersionedEntityMixin: versionedEntity,
			}), should.BeNil)

		t.Run("Cannot delete external group", func(t *ftt.Test) {
			request := &rpcpb.DeleteGroupRequest{
				Name: "mdb/foo",
			}

			_, err := srv.DeleteGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("cannot delete external group"))
		})

		t.Run("Group not found", func(t *ftt.Test) {
			request := &rpcpb.DeleteGroupRequest{
				Name: "non-existent-group",
			}

			_, err := srv.DeleteGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		})

		t.Run("Group referenced elsewhere", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{"owners"},
			})
			assert.Loosely(t, datastore.Put(ctx,
				&model.AuthGroup{
					ID:     "nesting-group",
					Parent: model.RootKey(ctx),
					Nested: []string{
						"test-group",
					},
				}), should.BeNil)
			request := &rpcpb.DeleteGroupRequest{
				Name: "test-group",
			}

			_, err := srv.DeleteGroup(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
		})

		t.Run("Permissions", func(t *ftt.Test) {

			t.Run("Anonymous is denied", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Normal user is denied", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
				})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})

			t.Run("Group owner succeeds", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{"owners"},
				})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("Admin succeeds", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity:       "user:someone@example.com",
					IdentityGroups: []string{model.AdminGroup},
				})
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("Etags", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{"owners"},
			})

			t.Run("Incorrect etag is aborted", func(t *ftt.Test) {
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
					Etag: "blah",
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Aborted))
			})

			t.Run("Empty etag succeeds", func(t *ftt.Test) {
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
				})
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("Correct etag succeeds if present", func(t *ftt.Test) {
				_, err := srv.DeleteGroup(ctx, &rpcpb.DeleteGroupRequest{
					Name: "test-group",
					Etag: etag,
				})
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})

	ftt.Run("Expansion of a group works", t, func(t *ftt.Test) {
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

		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := memory.Use(context.Background())
		assert.Loosely(t, datastore.Put(ctx,
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
				Description:              "This is a solo group",
				Owners:                   model.AdminGroup,
				CreatedTS:                createdTime,
				CreatedBy:                "user:test-solo-user@example.com",
				AuthVersionedEntityMixin: versionedEntity,
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
			}), should.BeNil)
		assert.Loosely(t, putRev(ctx, 123), should.BeNil)

		t.Run("GetExpandedGroup RPC call", func(t *ftt.Test) {
			t.Run("unknown group is not found", func(t *ftt.Test) {
				request := &rpcpb.GetGroupRequest{Name: "unknown"}
				_, err := srv.GetExpandedGroup(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})

			t.Run("works for standalone group", func(t *ftt.Test) {
				request := &rpcpb.GetGroupRequest{Name: soloGroup}
				expandedGroup, err := srv.GetExpandedGroup(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, expandedGroup, should.Match(&rpcpb.AuthGroup{
					Name:  soloGroup,
					Globs: []string{testGlob1},
				}))
			})

			t.Run("nested memberships returned", func(t *ftt.Test) {
				request := &rpcpb.GetGroupRequest{Name: owningGroup}
				expandedGroup, err := srv.GetExpandedGroup(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, expandedGroup, should.Match(&rpcpb.AuthGroup{
					Name:    owningGroup,
					Members: []string{testUser0, testUser1, testUser2},
					Globs:   []string{testGlob0, testGlob1},
					Nested:  []string{nestedGroup},
				}))
			})

			t.Run("works for nested group with no subgroups", func(t *ftt.Test) {
				request := &rpcpb.GetGroupRequest{Name: nestedGroup}
				expandedGroup, err := srv.GetExpandedGroup(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, expandedGroup, should.Match(&rpcpb.AuthGroup{
					Name:    nestedGroup,
					Members: []string{testUser2},
					Globs:   []string{testGlob0, testGlob1},
				}))
			})
		})

		t.Run("GetLegacyListing REST call", func(t *ftt.Test) {
			t.Run("empty name", func(t *ftt.Test) {
				rw := httptest.NewRecorder()
				rctx := &router.Context{
					Request: (&http.Request{}).WithContext(ctx),
					Params: []httprouter.Param{
						{Key: "groupName", Value: ""},
					},
					Writer: rw,
				}
				err := srv.GetLegacyListing(rctx)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
			})

			t.Run("unknown group is not found", func(t *ftt.Test) {
				rw := httptest.NewRecorder()
				rctx := &router.Context{
					Request: (&http.Request{}).WithContext(ctx),
					Params: []httprouter.Param{
						{Key: "groupName", Value: "test-group"},
					},
					Writer: rw,
				}
				err := srv.GetLegacyListing(rctx)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, rw.Body.Bytes(), should.BeEmpty)
			})

			t.Run("works for standalone group", func(t *ftt.Test) {
				rw := httptest.NewRecorder()
				rctx := &router.Context{
					Request: (&http.Request{}).WithContext(ctx),
					Params: []httprouter.Param{
						{Key: "groupName", Value: soloGroup},
					},
					Writer: rw,
				}
				err := srv.GetLegacyListing(rctx)
				assert.Loosely(t, err, should.BeNil)

				actual := map[string]ListingJSON{}
				assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
				assert.Loosely(t, actual, should.Match(map[string]ListingJSON{
					"listing": {
						Members: []listingPrincipal{},
						Globs: []listingPrincipal{
							{Principal: testGlob1},
						},
						Nested: []listingPrincipal{},
					},
				}))
			})

			t.Run("nested memberships returned", func(t *ftt.Test) {
				rw := httptest.NewRecorder()
				rctx := &router.Context{
					Request: (&http.Request{}).WithContext(ctx),
					Params: []httprouter.Param{
						{Key: "groupName", Value: owningGroup},
					},
					Writer: rw,
				}
				err := srv.GetLegacyListing(rctx)
				assert.Loosely(t, err, should.BeNil)

				actual := map[string]ListingJSON{}
				assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
				assert.Loosely(t, actual, should.Match(map[string]ListingJSON{
					"listing": {
						Members: []listingPrincipal{
							{Principal: testUser0},
							{Principal: testUser1},
							{Principal: testUser2},
						},
						Globs: []listingPrincipal{
							{Principal: testGlob0},
							{Principal: testGlob1},
						},
						Nested: []listingPrincipal{
							{Principal: nestedGroup},
						},
					},
				}))
			})

			t.Run("works for nested group with no subgroup", func(t *ftt.Test) {
				rw := httptest.NewRecorder()
				rctx := &router.Context{
					Request: (&http.Request{}).WithContext(ctx),
					Params: []httprouter.Param{
						{Key: "groupName", Value: nestedGroup},
					},
					Writer: rw,
				}
				err := srv.GetLegacyListing(rctx)
				assert.Loosely(t, err, should.BeNil)

				actual := map[string]ListingJSON{}
				assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
				assert.Loosely(t, actual, should.Match(map[string]ListingJSON{
					"listing": {
						Members: []listingPrincipal{
							{Principal: testUser2},
						},
						Globs: []listingPrincipal{
							{Principal: testGlob0},
							{Principal: testGlob1},
						},
						Nested: []listingPrincipal{},
					},
				}))
			})
		})
	})

	ftt.Run("GetSubgraph RPC call", t, func(t *ftt.Test) {
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

		srv := Server{
			authGroupsProvider: &CachingGroupsProvider{},
		}
		ctx := memory.Use(context.Background())
		assert.Loosely(t, datastore.Put(ctx,
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
			}), should.BeNil)
		assert.Loosely(t, putRev(ctx, 123), should.BeNil)

		t.Run("Identity principal", func(t *ftt.Test) {
			request := &rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_IDENTITY,
					Name: testUser0,
				},
			}

			actualSubgraph, err := srv.GetSubgraph(ctx, request)
			assert.Loosely(t, err, should.BeNil)

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

			assert.Loosely(t, actualSubgraph, should.Match(expectedSubgraph))
		})

		t.Run("Group principal", func(t *ftt.Test) {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: nestedGroup,
				},
			}

			actualSubgraph, err := srv.GetSubgraph(ctx, &request)
			assert.Loosely(t, err, should.BeNil)

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

			assert.Loosely(t, actualSubgraph, should.Match(expectedSubgraph))

		})

		t.Run("Glob principal", func(t *ftt.Test) {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GLOB,
					Name: testGlob1,
				},
			}

			actualSubgraph, err := srv.GetSubgraph(ctx, &request)
			assert.Loosely(t, err, should.BeNil)

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

			assert.Loosely(t, actualSubgraph, should.Match(expectedSubgraph))
		})

		t.Run("Unspecified Principal kind", func(t *ftt.Test) {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_PRINCIPAL_KIND_UNSPECIFIED,
					Name: "aeua//",
				},
			}

			_, err := srv.GetSubgraph(ctx, &request)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid principal kind"))

		})

		t.Run("Group principal not in groups graph", func(t *ftt.Test) {
			request := rpcpb.GetSubgraphRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: "i-dont-exist",
				},
			}

			_, err := srv.GetSubgraph(ctx, &request)
			assert.Loosely(t, err.Error(), should.ContainSubstring("not found"))
		})
	})
}
