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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestGetRootInvocation(t *testing.T) {
	ftt.Run("GetRootInvocation", t, func(t *ftt.Test) {
		const realm = "testproject:testrealm"

		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv-id")
		rootWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       workunits.RootWorkUnitID,
		}

		// Setup authorisation.
		token, err := generateWorkUnitUpdateToken(ctx, rootWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		req := &pb.GetRootInvocationRequest{
			Name: "rootInvocations/root-inv-id",
		}

		recorder := newTestRecorderServer()

		t.Run("happy path", func(t *ftt.Test) {
			// Insert a root invocation.
			testData := rootinvocations.NewBuilder(rootInvID).
				WithFinalizationState(pb.RootInvocation_FINALIZED).
				WithRealm(realm).Build()
			testutil.MustApply(ctx, t, insert.RootInvocationOnly(testData)...)

			expectedRootInvocation := &pb.RootInvocation{
				Name:                 "rootInvocations/root-inv-id",
				RootInvocationId:     "root-inv-id",
				Realm:                realm,
				FinalizationState:    pb.RootInvocation_FINALIZED,
				State:                pb.RootInvocation_FAILED,
				SummaryMarkdown:      "The FooBar returned false when it was expected to return true.",
				CreateTime:           pbutil.MustTimestampProto(testData.CreateTime),
				Creator:              testData.CreatedBy,
				LastUpdated:          pbutil.MustTimestampProto(testData.LastUpdated),
				FinalizeTime:         pbutil.MustTimestampProto(testData.FinalizeTime.Time),
				FinalizeStartTime:    pbutil.MustTimestampProto(testData.FinalizeStartTime.Time),
				ProducerResource:     testData.ProducerResource,
				Definition:           testData.Definition,
				Sources:              testData.Sources,
				PrimaryBuild:         testData.PrimaryBuild,
				ExtraBuilds:          testData.ExtraBuilds,
				Tags:                 testData.Tags,
				Properties:           testData.Properties,
				BaselineId:           testData.BaselineID,
				StreamingExportState: testData.StreamingExportState,
				Etag:                 `W/"2025-04-26T01:02:03.000004Z"`,
			}

			rsp, err := recorder.GetRootInvocation(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rsp, should.Match(expectedRootInvocation))
		})

		t.Run("does not exist", func(t *ftt.Test) {
			_, err := recorder.GetRootInvocation(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id" not found`))
		})

		t.Run("request authorization", func(t *ftt.Test) {
			t.Run("invalid update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid-token"))
				_, err := recorder.GetRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.That(t, err, should.ErrLike(`desc = invalid update token`))
			})
			t.Run("missing update token", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.MD{})
				_, err := recorder.GetRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.Unauthenticated))
				assert.That(t, err, should.ErrLike(`desc = missing update-token metadata value in the request`))
			})
		})

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("name", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Name = "invalid-name"
					_, err := recorder.GetRootInvocation(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: does not match"))
				})
				t.Run("empty", func(t *ftt.Test) {
					req.Name = ""
					_, err := recorder.GetRootInvocation(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: unspecified"))
				})
			})
		})
	})
}
