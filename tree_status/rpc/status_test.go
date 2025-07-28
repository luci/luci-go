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
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/config"
	"go.chromium.org/luci/tree_status/internal/perms"
	"go.chromium.org/luci/tree_status/internal/status"
	"go.chromium.org/luci/tree_status/internal/testutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"
)

func TestStatus(t *testing.T) {
	ftt.Run("With a Status server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)

		// For config.
		ctx = memory.Use(ctx)
		testConfig := config.TestConfig()
		err := config.SetConfig(ctx, testConfig)
		assert.Loosely(t, err, should.BeNil)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-tree-status-access", "googlers"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewTreeStatusServer()
		t.Run("GetStatus", func(t *ftt.Test) {
			t.Run("Default ACLs anonymous rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().Anonymous().SetInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("log in"))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Default ACLs no read access rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().SetInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user is not a member of group \"luci-tree-status-access\""))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Realm-based ACLs no read access rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().SetInContext(ctx)
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user does not have permission to perform this action"))
				assert.Loosely(t, status, should.BeNil)
			})

			t.Run("Read latest when no status updates returns fallback", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithAuditAccess().SetInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status.Name, should.Equal("trees/chromium/status/fallback"))
				assert.Loosely(t, status.GeneralState, should.Equal(pb.GeneralState_OPEN))
				assert.Loosely(t, status.Message, should.Equal("Tree is open (fallback due to no status updates in past 140 days)"))
			})
			t.Run("Default ACLs read latest", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithAuditAccess().SetInContext(ctx)
				NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", latest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "latest",
					CreateUser:   "someone@example.com",
					CreateTime:   timestamppb.New(latest.CreateTime),
				}))
			})

			t.Run("Realm-based ACLs read latest", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermGetStatus, "chromium:@project").WithPermissionInRealm(perms.PermGetStatusLimited, "chromium:@project").WithAuditAccess().SetInContext(ctx)
				NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", latest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "latest",
					CreateUser:   "someone@example.com",
					CreateTime:   timestamppb.New(latest.CreateTime),
				}))
			})

			t.Run("Default ACLs read latest with no audit access hides username", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)
				NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", latest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "latest",
					CreateTime:   timestamppb.New(latest.CreateTime),
				}))
			})

			t.Run("Realm-based ACLs read latest with PermGetStatusLimited access hides username", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermGetStatusLimited, "chromium:@project").SetInContext(ctx)
				NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", latest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "latest",
					CreateTime:   timestamppb.New(latest.CreateTime),
				}))
			})

			t.Run("Realm-based ACLs read latest without audit access access hides username", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermGetStatus, "chromium:@project").WithPermissionInRealm(perms.PermGetStatusLimited, "chromium:@project").SetInContext(ctx)
				NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/latest",
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", latest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "latest",
					CreateTime:   timestamppb.New(latest.CreateTime),
				}))
			})

			t.Run("Default ACLs read by name", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithAuditAccess().SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "earliest",
					CreateUser:   "someone@example.com",
					CreateTime:   timestamppb.New(earliest.CreateTime),
				}))
			})

			t.Run("Realm-based ACLs read by name", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermGetStatus, "chromium:@project").WithPermissionInRealm(perms.PermGetStatusLimited, "chromium:@project").WithAuditAccess().SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "earliest",
					CreateUser:   "someone@example.com",
					CreateTime:   timestamppb.New(earliest.CreateTime),
				}))
			})

			t.Run("Default ACLs read by name with no audit access hides username", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "earliest",
					CreateTime:   timestamppb.New(earliest.CreateTime),
				}))
			})

			t.Run("Realm-based ACLs read by name with PermGetStatusLimited permission hides username", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermGetStatusLimited, "chromium:@project").SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "earliest",
					CreateTime:   timestamppb.New(earliest.CreateTime),
				}))
			})

			t.Run("Realm-based ACLs read by name with no audit access hides username", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermGetStatus, "chromium:@project").WithPermissionInRealm(perms.PermGetStatusLimited, "chromium:@project").SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.GetStatusRequest{
					Name: fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
				}
				status, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, status, should.Match(&pb.Status{
					Name:         fmt.Sprintf("trees/chromium/status/%s", earliest.StatusID),
					GeneralState: pb.GeneralState_OPEN,
					Message:      "earliest",
					CreateTime:   timestamppb.New(earliest.CreateTime),
				}))
			})

			t.Run("Read of invalid id", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/INVALID",
				}
				_, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("name: expected format"))
			})
			t.Run("Read of non existing valid id", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)

				request := &pb.GetStatusRequest{
					Name: "trees/chromium/status/abcd1234abcd1234abcd1234abcd1234",
				}
				_, err := server.GetStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("status value was not found"))
			})
		})

		t.Run("Create", func(t *ftt.Test) {
			t.Run("Default ACLs anonymous rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().Anonymous().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("log in"))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Default ACLs no access rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user is not a member of group \"luci-tree-status-writers\""))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Default ACLs no write access rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user is not a member of group \"luci-tree-status-writers\""))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Realm-based ACLs no write access rejected", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user does not have permission to perform this action"))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Default ACLs successful create", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState:       pb.GeneralState_CLOSED,
						Message:            "closed",
						ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual, should.Match(&pb.Status{
					Name:               actual.Name,
					GeneralState:       pb.GeneralState_CLOSED,
					Message:            "closed",
					CreateUser:         "someone@example.com",
					CreateTime:         actual.CreateTime,
					ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
				}))
				assert.Loosely(t, actual.Name, should.ContainSubstring("trees/chromium/status/"))
				assert.Loosely(t, time.Since(actual.CreateTime.AsTime()), should.BeLessThan(time.Minute))

				// Check it was actually written to the DB.
				s, err := status.ReadLatest(span.Single(ctx), "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match(&status.Status{
					TreeName:           "chromium",
					StatusID:           strings.Split(actual.Name, "/")[3],
					GeneralStatus:      pb.GeneralState_CLOSED,
					Message:            "closed",
					CreateUser:         "someone@example.com",
					CreateTime:         actual.CreateTime.AsTime(),
					ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
				}))
			})

			t.Run("Realm-based ACLs successful create", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermCreateStatus, "chromium:@project").SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState:       pb.GeneralState_CLOSED,
						Message:            "closed",
						ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual, should.Match(&pb.Status{
					Name:               actual.Name,
					GeneralState:       pb.GeneralState_CLOSED,
					Message:            "closed",
					CreateUser:         "someone@example.com",
					CreateTime:         actual.CreateTime,
					ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
				}))
				assert.Loosely(t, actual.Name, should.ContainSubstring("trees/chromium/status/"))
				assert.Loosely(t, time.Since(actual.CreateTime.AsTime()), should.BeLessThan(time.Minute))

				// Check it was actually written to the DB.
				s, err := status.ReadLatest(span.Single(ctx), "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match(&status.Status{
					TreeName:           "chromium",
					StatusID:           strings.Split(actual.Name, "/")[3],
					GeneralStatus:      pb.GeneralState_CLOSED,
					Message:            "closed",
					CreateUser:         "someone@example.com",
					CreateTime:         actual.CreateTime.AsTime(),
					ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
				}))
			})

			t.Run("Closing builder name ignored if status is not closed", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState:       pb.GeneralState_OPEN,
						Message:            "open",
						ClosingBuilderName: "projects/chromium-m100/buckets/ci.shadow/builders/Linux 123",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual, should.Match(&pb.Status{
					Name:         actual.Name,
					GeneralState: pb.GeneralState_OPEN,
					Message:      "open",
					CreateUser:   "someone@example.com",
					CreateTime:   actual.CreateTime,
				}))
				assert.Loosely(t, actual.Name, should.ContainSubstring("trees/chromium/status/"))
				assert.Loosely(t, time.Since(actual.CreateTime.AsTime()), should.BeLessThan(time.Minute))

				// Check it was actually written to the DB.
				s, err := status.ReadLatest(span.Single(ctx), "chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match(&status.Status{
					TreeName:      "chromium",
					StatusID:      strings.Split(actual.Name, "/")[3],
					GeneralStatus: pb.GeneralState_OPEN,
					Message:       "open",
					CreateUser:    "someone@example.com",
					CreateTime:    actual.CreateTime.AsTime(),
				}))
			})
			t.Run("Invalid parent", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "chromium",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
					},
				}
				_, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("expected format: ^trees/"))
			})
			t.Run("Name ignored", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						Name:         "incorrect_format_name",
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual.Name, should.ContainSubstring("trees/chromium/status/"))
			})
			t.Run("Empty general_state", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						Message: "closed",
					},
				}
				_, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("general_state: must be specified"))
			})
			t.Run("Empty message", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
					},
				}
				_, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("message: must be specified"))
			})
			t.Run("User ignored", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
						CreateUser:   "someoneelse@somewhere.com",
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, actual.CreateUser, should.Equal("someone@example.com"))
			})
			t.Run("Time ignored", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithWriteAccess().SetInContext(ctx)

				request := &pb.CreateStatusRequest{
					Parent: "trees/chromium/status",
					Status: &pb.Status{
						GeneralState: pb.GeneralState_CLOSED,
						Message:      "closed",
						CreateTime:   timestamppb.New(time.Time{}),
					},
				}
				actual, err := server.CreateStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, time.Since(actual.CreateTime.AsTime()), should.BeLessThan(time.Minute))
			})
		})

		t.Run("List", func(t *ftt.Test) {
			t.Run("Default ACLs anonymous rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().Anonymous().SetInContext(ctx)

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("log in"))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Default ACLs no read access rejected", func(t *ftt.Test) {
				ctx = perms.FakeAuth().SetInContext(ctx)

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user is not a member of group \"luci-tree-status-access\""))
				assert.Loosely(t, status, should.BeNil)
			})
			t.Run("Realm-based ACLs no read access rejected", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().SetInContext(ctx)

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				status, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("user does not have permission to perform this action"))
				assert.Loosely(t, status, should.BeNil)
			})

			t.Run("List with no values", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithAuditAccess().SetInContext(ctx)

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				response, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response.NextPageToken, should.BeEmpty)
				assert.Loosely(t, response.Status, should.BeEmpty)
			})
			t.Run("List with one page", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithAuditAccess().SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.ListStatusRequest{
					Parent:   "trees/chromium/status",
					PageSize: 2,
				}
				response, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response.NextPageToken, should.BeEmpty)
				assert.Loosely(t, response, should.Match(&pb.ListStatusResponse{
					Status: []*pb.Status{
						toStatusProto(latest, true),
						toStatusProto(earliest, true),
					},
				}))
			})
			t.Run("List with two pages", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().WithAuditAccess().SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.ListStatusRequest{
					Parent:   "trees/chromium/status",
					PageSize: 1,
				}
				response1, err := server.ListStatus(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				request.PageToken = response1.NextPageToken
				response2, err := server.ListStatus(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, response1.NextPageToken, should.NotBeEmpty)
				assert.Loosely(t, response2.NextPageToken, should.BeEmpty)
				assert.Loosely(t, response1.Status[0], should.Match(toStatusProto(latest, true)))
				assert.Loosely(t, response2.Status[0], should.Match(toStatusProto(earliest, true)))
			})

			t.Run("Realm-based ACLs list", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermListStatusLimited, "chromium:@project").WithPermissionInRealm(perms.PermListStatus, "chromium:@project").WithAuditAccess().SetInContext(ctx)
				earliest := NewStatusBuilder().WithMessage("earliest").CreateInDB(ctx)
				latest := NewStatusBuilder().WithMessage("latest").CreateInDB(ctx)

				request := &pb.ListStatusRequest{
					Parent:   "trees/chromium/status",
					PageSize: 2,
				}
				response, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response.NextPageToken, should.BeEmpty)
				assert.Loosely(t, response, should.Match(&pb.ListStatusResponse{
					Status: []*pb.Status{
						toStatusProto(latest, true),
						toStatusProto(earliest, true),
					},
				}))
			})

			t.Run("Default ACLs list with no audit access hides usernames", func(t *ftt.Test) {
				ctx = perms.FakeAuth().WithReadAccess().SetInContext(ctx)
				expected := toStatusProto(NewStatusBuilder().CreateInDB(ctx), true)
				expected.CreateUser = ""

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				response, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.ListStatusResponse{
					Status: []*pb.Status{expected},
				}))
			})

			t.Run("Realm-based ACLs list with PermListStatusLimited hides usernames", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermListStatusLimited, "chromium:@project").SetInContext(ctx)
				expected := toStatusProto(NewStatusBuilder().CreateInDB(ctx), true)
				expected.CreateUser = ""

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				response, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.ListStatusResponse{
					Status: []*pb.Status{expected},
				}))
			})

			t.Run("Realm-based ACLs list with no audit access hides usernames", func(t *ftt.Test) {
				testConfig.Trees[0].UseDefaultAcls = false
				err := config.SetConfig(ctx, testConfig)
				assert.Loosely(t, err, should.BeNil)

				ctx = perms.FakeAuth().WithPermissionInRealm(perms.PermListStatusLimited, "chromium:@project").WithPermissionInRealm(perms.PermListStatus, "chromium:@project").SetInContext(ctx)
				expected := toStatusProto(NewStatusBuilder().CreateInDB(ctx), true)
				expected.CreateUser = ""

				request := &pb.ListStatusRequest{
					Parent: "trees/chromium/status",
				}
				response, err := server.ListStatus(ctx, request)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.ListStatusResponse{
					Status: []*pb.Status{expected},
				}))
			})
		})
	})
}

type StatusBuilder struct {
	status status.Status
}

func NewStatusBuilder() *StatusBuilder {
	id, err := status.GenerateID()
	if err != nil {
		panic(err)
	}
	return &StatusBuilder{status: status.Status{
		TreeName:      "chromium",
		StatusID:      id,
		GeneralStatus: pb.GeneralState_OPEN,
		Message:       "Tree is open!",
		CreateUser:    "someone@example.com",
		CreateTime:    spanner.CommitTimestamp,
	}}
}

func (b *StatusBuilder) WithMessage(message string) *StatusBuilder {
	b.status.Message = message
	return b
}

func (b *StatusBuilder) Build() *status.Status {
	s := b.status
	return &s
}

func (b *StatusBuilder) CreateInDB(ctx context.Context) *status.Status {
	s := b.Build()
	row := map[string]any{
		"TreeName":      s.TreeName,
		"StatusId":      s.StatusID,
		"GeneralStatus": int64(s.GeneralStatus),
		"Message":       s.Message,
		"CreateUser":    s.CreateUser,
		"CreateTime":    s.CreateTime,
	}
	m := spanner.InsertOrUpdateMap("Status", row)
	ts, err := span.Apply(ctx, []*spanner.Mutation{m})
	if err != nil {
		panic(err)
	}
	if s.CreateTime == spanner.CommitTimestamp {
		s.CreateTime = ts.UTC()
	}
	return s
}
