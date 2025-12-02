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

package recorder

import (
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestGetWorkUnit(t *testing.T) {
	ftt.Run("GetWorkUnit", t, func(t *ftt.Test) {
		const (
			rootRealm = "testproject:root"
			wuRealm   = "testproject:workunit"
		)
		rootInvID := rootinvocations.ID("root-inv-id")
		rootWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "root",
		}

		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // For config datastore cache.

		// Setup service config.
		cfg := config.CreatePlaceholderServiceConfig()
		err := config.SetServiceConfigForTesting(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		// Insert a root invocation and a work unit.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm(rootRealm).Build()
		rootWu := workunits.NewBuilder(rootInvID, "root").WithRealm(wuRealm).Build()
		testutil.MustApply(ctx, t, insert.RootInvocationOnly(rootInv)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(rootWu)...)

		// Insert a child work unit.
		childWuID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "child-work-unit-id",
		}
		childWu := workunits.NewBuilder(rootInvID, childWuID.WorkUnitID).
			WithRealm(wuRealm).
			WithParentWorkUnitID(rootWorkUnitID.WorkUnitID).
			WithMinimalFields().
			Build()
		testutil.MustApply(ctx, t, insert.WorkUnit(childWu)...)

		// Setup authorisation.
		token, err := generateWorkUnitUpdateToken(ctx, rootWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		req := &pb.GetWorkUnitRequest{
			Name: rootWorkUnitID.Name(),
		}

		recorder := newTestRecorderServer()

		t.Run("happy path", func(t *ftt.Test) {
			expectedRsp := &pb.WorkUnit{
				Name:              rootWorkUnitID.Name(),
				WorkUnitId:        rootWorkUnitID.WorkUnitID,
				Kind:              rootWu.Kind,
				State:             rootWu.State,
				FinalizationState: rootWu.FinalizationState,
				SummaryMarkdown:   rootWu.SummaryMarkdown,
				Realm:             rootWu.Realm,
				CreateTime:        pbutil.MustTimestampProto(rootWu.CreateTime),
				Creator:           rootWu.CreatedBy,
				LastUpdated:       pbutil.MustTimestampProto(rootWu.LastUpdated),
				FinalizeStartTime: pbutil.MustTimestampProto(rootWu.FinalizeStartTime.Time),
				FinalizeTime:      pbutil.MustTimestampProto(rootWu.FinalizeTime.Time),
				Deadline:          pbutil.MustTimestampProto(rootWu.Deadline),
				Parent:            rootInvID.Name(),
				ChildWorkUnits:    []string{childWuID.Name()},
				ModuleId: &pb.ModuleIdentifier{
					ModuleName:        rootWu.ModuleID.ModuleName,
					ModuleScheme:      rootWu.ModuleID.ModuleScheme,
					ModuleVariant:     rootWu.ModuleID.ModuleVariant,
					ModuleVariantHash: rootWu.ModuleID.ModuleVariantHash,
				},
				ModuleShardKey: rootWu.ModuleShardKey,
				ProducerResource: &pb.ProducerResource{
					System:    "buildbucket",
					DataRealm: "prod",
					Name:      "builds/123",
					Url:       "https://milo-prod/ui/b/123",
				},
				Tags:         rootWu.Tags,
				Properties:   rootWu.Properties,
				Instructions: rootWu.Instructions,
				IsMasked:     false,
				Etag:         `W/"/2025-04-26T01:02:03.000004Z"`,
			}

			t.Run("default view", func(t *ftt.Test) {
				rsp, err := recorder.GetWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, rsp, should.Match(expectedRsp))
			})
			t.Run("basic view", func(t *ftt.Test) {
				req.View = pb.WorkUnitView_WORK_UNIT_VIEW_BASIC

				rsp, err := recorder.GetWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, rsp, should.Match(expectedRsp))
			})
			t.Run("full view", func(t *ftt.Test) {
				req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
				expectedRsp.ExtendedProperties = rootWu.ExtendedProperties
				expectedRsp.Etag = `W/"+f/2025-04-26T01:02:03.000004Z"`

				rsp, err := recorder.GetWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, rsp, should.Match(expectedRsp))
			})
		})

		t.Run("does not exist", func(t *ftt.Test) {
			testutil.MustApply(ctx, t, spanner.Delete("WorkUnits", rootWorkUnitID.Key()))

			_, err := recorder.GetWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`desc = "rootInvocations/root-inv-id/workUnits/root" not found`))
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.GetWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`desc = invalid update token`))
			})
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.GetWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`desc = missing update-token metadata value in the request`))
			})
		})

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("name", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Name = "invalid-name"
					_, err := recorder.GetWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("bad request: name: does not match"))
				})
				t.Run("empty", func(t *ftt.Test) {
					req.Name = ""
					_, err := recorder.GetWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("bad request: name: unspecified"))
				})
			})
			t.Run("view", func(t *ftt.Test) {
				req.Name = rootWorkUnitID.Name()
				req.View = pb.WorkUnitView(999)
				_, err := recorder.GetWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("bad request: view: unrecognized view"))
			})
		})
	})
}
