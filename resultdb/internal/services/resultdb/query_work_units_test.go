// Copyright 2025 The LUCI Authors.
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

package resultdb

import (
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryWorkUnits(t *testing.T) {
	ftt.Run("QueryWorkUnits", t, func(t *ftt.Test) {
		const (
			rootInvRealm = "testproject:root_inv"
			fullRealm    = "testproject:full_access"
			limitedRealm = "testproject:limited_access"
		)
		rootInvID := rootinvocations.ID("root-inv-id")

		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		// Setup service config.
		cfg := config.CreatePlaceholderServiceConfig()
		err := config.SetServiceConfigForTesting(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		compiledCfg, err := config.NewCompiledServiceConfig(config.CreatePlaceholderServiceConfig(), "revision")
		assert.NoErr(t, err)

		// Insert a root invocation.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm(rootInvRealm).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationOnly(rootInv)...)

		// Insert some work units
		rootWu := workunits.NewBuilder(rootInvID, "root").WithRealm(fullRealm).Build()
		extraProps, _ := structpb.NewStruct(map[string]any{"key": "value"})
		ep := map[string]*structpb.Struct{"mykey": extraProps}
		wu1 := workunits.NewBuilder(rootInvID, "wu1").WithRealm(limitedRealm).WithExtendedProperties(ep).Build()
		rootWu.ChildWorkUnits = []workunits.ID{wu1.ID}

		testutil.MustApply(ctx, t, workunits.InsertForTesting(rootWu)...)
		testutil.MustApply(ctx, t, workunits.InsertForTesting(wu1)...)

		// Default authorisation: Full access to everything via Root Invocation realm.
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: rootInvRealm, Permission: rdbperms.PermListLimitedWorkUnits},
				{Realm: rootInvRealm, Permission: rdbperms.PermListWorkUnits},
			},
		}
		ctx = auth.WithState(ctx, authState)

		srv := newTestResultDBService()
		baseReq := &pb.QueryWorkUnitsRequest{
			Parent: rootInvID.Name(),
		}

		expectedRootWU := masking.WorkUnit(rootWu, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg)
		expectedWU1 := masking.WorkUnit(wu1, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, compiledCfg)

		t.Run("happy path", func(t *ftt.Test) {
			expectedRootWUMasked := proto.Clone(expectedRootWU).(*pb.WorkUnit)
			expectedRootWUMasked.Etag = `W/"+l/2025-04-26T01:02:03.000004Z"`
			expectedRootWUMasked.ModuleId.ModuleVariant = nil
			expectedRootWUMasked.Tags = nil
			expectedRootWUMasked.Properties = nil
			expectedRootWUMasked.Instructions = nil
			expectedRootWUMasked.ExtendedProperties = nil
			expectedRootWUMasked.IsMasked = true

			expectedWU1Masked := proto.Clone(expectedWU1).(*pb.WorkUnit)
			expectedWU1Masked.Etag = `W/"+l/2025-04-26T01:02:03.000004Z"`
			expectedWU1Masked.ModuleId.ModuleVariant = nil
			expectedWU1Masked.Tags = nil
			expectedWU1Masked.Properties = nil
			expectedWU1Masked.Instructions = nil
			expectedWU1Masked.ExtendedProperties = nil
			expectedWU1Masked.IsMasked = true
			t.Run("full access", func(t *ftt.Test) {
				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.QueryWorkUnits(ctx, baseReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1, expectedRootWU}))
				})
				t.Run("basic view", func(t *ftt.Test) {
					baseReq.View = pb.WorkUnitView_WORK_UNIT_VIEW_BASIC

					rsp, err := srv.QueryWorkUnits(ctx, baseReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1, expectedRootWU}))
				})
				t.Run("full view", func(t *ftt.Test) {
					baseReq.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
					expectedRootWU.ExtendedProperties = rootWu.ExtendedProperties
					expectedRootWU.Etag = `W/"+f/2025-04-26T01:02:03.000004Z"`
					expectedWU1.ExtendedProperties = wu1.ExtendedProperties
					expectedWU1.Etag = `W/"+f/2025-04-26T01:02:03.000004Z"`

					rsp, err := srv.QueryWorkUnits(ctx, baseReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1, expectedRootWU}))
				})

				t.Run("masked access upgraded to full", func(t *ftt.Test) {
					ctx := auth.WithState(ctx, &authtest.FakeState{
						Identity: "user:someone@example.com",
						IdentityPermissions: []authtest.RealmPermission{
							// Only root work unit is upgraded to full access.
							{Realm: rootWu.Realm, Permission: rdbperms.PermGetWorkUnit},
							{Realm: rootInvRealm, Permission: rdbperms.PermListLimitedWorkUnits},
						},
					})

					rsp, err := srv.QueryWorkUnits(ctx, baseReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1Masked, expectedRootWU}))
				})
			})
			t.Run("limited access", func(t *ftt.Test) {
				ctx := auth.WithState(ctx, &authtest.FakeState{
					Identity: "user:someone@example.com",
					IdentityPermissions: []authtest.RealmPermission{
						{Realm: rootInvRealm, Permission: rdbperms.PermListLimitedWorkUnits},
					},
				})
				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.QueryWorkUnits(ctx, baseReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1Masked, expectedRootWUMasked}))
				})
				t.Run("basic view", func(t *ftt.Test) {
					baseReq.View = pb.WorkUnitView_WORK_UNIT_VIEW_BASIC

					rsp, err := srv.QueryWorkUnits(ctx, baseReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1Masked, expectedRootWUMasked}))
				})
				t.Run("full view", func(t *ftt.Test) {
					baseReq.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
					expectedRootWUMasked.Etag = `W/"+l+f/2025-04-26T01:02:03.000004Z"`
					expectedWU1Masked.Etag = `W/"+l+f/2025-04-26T01:02:03.000004Z"`

					rsp, err := srv.QueryWorkUnits(ctx, baseReq)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1Masked, expectedRootWUMasked}))
				})
			})

			t.Run("with ancestors_of predicate", func(t *ftt.Test) {
				baseReq.Predicate = &pb.WorkUnitPredicate{
					AncestorsOf: wu1.ID.Name(),
				}

				rsp, err := srv.QueryWorkUnits(ctx, baseReq)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
				assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedRootWU}))
			})
		})

		t.Run("pagination", func(t *ftt.Test) {
			t.Run("by page size", func(t *ftt.Test) {
				baseReq.PageSize = 1
				rsp, err := srv.QueryWorkUnits(ctx, baseReq)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)
				assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWU1}))

				// Next page.
				baseReq.PageToken = rsp.NextPageToken
				rsp, err = srv.QueryWorkUnits(ctx, baseReq)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)
				assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedRootWU}))

				// Last page
				baseReq.PageToken = rsp.NextPageToken
				rsp, err = srv.QueryWorkUnits(ctx, baseReq)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
				assert.Loosely(t, rsp.WorkUnits, should.BeNil)
			})

			t.Run("by response size limit", func(t *ftt.Test) {
				largeRootInvID := rootinvocations.ID("large-inv-id")
				rootInv := rootinvocations.NewBuilder(largeRootInvID).WithRealm(rootInvRealm).Build()
				testutil.MustApply(ctx, t, insert.RootInvocationOnly(rootInv)...)

				// Create work units with large extended properties.
				largeValue := strings.Repeat("a", 20*1000*1000+1)
				largeEP, _ := structpb.NewStruct(map[string]any{"key": largeValue})
				largeEPMap := map[string]*structpb.Struct{"large": largeEP}

				largeWU1 := workunits.NewBuilder(largeRootInvID, "root").
					WithRealm(fullRealm).
					WithExtendedProperties(largeEPMap).
					Build()

				largeWU2 := workunits.NewBuilder(largeRootInvID, "large-wu-2").
					WithRealm(fullRealm).
					WithExtendedProperties(largeEPMap).
					Build()
				largeWU1.ChildWorkUnits = []workunits.ID{largeWU2.ID}
				expectedLargeWU1 := masking.WorkUnit(largeWU1, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL, compiledCfg)
				expectedLargeWU2 := masking.WorkUnit(largeWU2, permissions.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL, compiledCfg)

				testutil.MustApply(ctx, t, workunits.InsertForTesting(largeWU1)...)
				testutil.MustApply(ctx, t, workunits.InsertForTesting(largeWU2)...)

				req := &pb.QueryWorkUnitsRequest{
					Parent: largeRootInvID.Name(),
					View:   pb.WorkUnitView_WORK_UNIT_VIEW_FULL,
				}
				t.Run("paginate supported", func(t *ftt.Test) {
					rsp, err := srv.QueryWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedLargeWU1}))

					// Next page
					req.PageToken = rsp.NextPageToken
					rsp, err = srv.QueryWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.NotBeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedLargeWU2}))

					// Next page
					req.PageToken = rsp.NextPageToken
					rsp, err = srv.QueryWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.NextPageToken, should.BeEmpty)
					assert.Loosely(t, rsp.WorkUnits, should.BeNil)
				})

				t.Run("with ancestor_of predicate", func(t *ftt.Test) {
					req.Predicate = &pb.WorkUnitPredicate{AncestorsOf: largeWU2.ID.Name()}
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.Loosely(t, err, should.NotBeNil)
					assert.That(t, err, grpccode.ShouldBe(codes.Unimplemented))
					assert.That(t, err, should.ErrLike("pagination is not supported for ancestors_of queries"))
				})
			})
		})

		t.Run("root invocation doesn't exist", func(t *ftt.Test) {
			baseReq.Parent = "rootInvocations/non-existent"

			_, err := srv.QueryWorkUnits(ctx, baseReq)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike("desc = \"rootInvocations/non-existent\" not found"))
		})

		t.Run("with predicate - start node does not exist", func(t *ftt.Test) {
			req := baseReq
			req.Predicate = &pb.WorkUnitPredicate{
				AncestorsOf: workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "non-existent"}.Name(),
			}

			_, err := srv.QueryWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/non-existent" not found`))
		})

		t.Run("request authorization", func(t *ftt.Test) {
			req := baseReq
			authState.IdentityPermissions = nil
			ctx = auth.WithState(ctx, authState)
			_, err := srv.QueryWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike(`caller does not have permission resultdb.workUnits.list (or resultdb.workUnits.listLimited) in realm of root invocation "rootInvocations/root-inv-id"`))
		})

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := baseReq
					req.Parent = ""
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("parent: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req := baseReq
					req.Parent = "invalid"
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("parent: does not match pattern"))
				})
			})

			t.Run("view", func(t *ftt.Test) {
				req := baseReq
				req.View = pb.WorkUnitView(999)
				_, err := srv.QueryWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("view: unrecognized view"))
			})

			t.Run("page size", func(t *ftt.Test) {
				baseReq.PageSize = -1

				_, err := srv.QueryWorkUnits(ctx, baseReq)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("page_size: negative"))
			})

			t.Run("page token", func(t *ftt.Test) {
				baseReq.PageToken = "invalid"

				_, err := srv.QueryWorkUnits(ctx, baseReq)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("invalid page_token"))
			})

			t.Run("predicate", func(t *ftt.Test) {
				req := baseReq
				t.Run("ancestors_of unspecified", func(t *ftt.Test) {
					req.Predicate = &pb.WorkUnitPredicate{}
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("predicate: ancestors_of: unspecified"))
				})
				t.Run("ancestors_of invalid format", func(t *ftt.Test) {
					req.Predicate = &pb.WorkUnitPredicate{AncestorsOf: "invalid"}
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("predicate: ancestors_of: does not match pattern"))
				})
				t.Run("ancestors_of wrong root", func(t *ftt.Test) {
					req.Predicate = &pb.WorkUnitPredicate{
						AncestorsOf: workunits.ID{RootInvocationID: "other-root", WorkUnitID: "child"}.Name(),
					}
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("does not belong to the parent root invocation"))
				})
			})
		})
	})
}
