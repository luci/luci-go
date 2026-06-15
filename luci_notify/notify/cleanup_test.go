// Copyright 2026 The LUCI Authors.
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

package notify

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	buildbucketgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/luci_notify/internal/testutil"
)

func TestCleanupStaleBuilders(t *testing.T) {
	ftt.Run("CleanupStaleBuilders", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		mockCtrl := gomock.NewController(t)
		mockBuildersClient := buildbucketgrpcpb.NewMockBuildersClient(mockCtrl)

		// Override the client creator to return our mock client.
		oldCreator := buildersClientCreator
		buildersClientCreator = func(ctx context.Context) (buildbucketgrpcpb.BuildersClient, error) {
			return mockBuildersClient, nil
		}
		t.Cleanup(func() {
			buildersClientCreator = oldCreator
		})

		// Helper to insert builder statuses into Spanner.
		insertBuilders := func(builders []map[string]any) {
			var ms []*spanner.Mutation
			for _, b := range builders {
				m := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
					"BuilderKey":      b["BuilderKey"],
					"Project":         b["Project"],
					"Bucket":          b["Bucket"],
					"Builder":         b["Builder"],
					"Realm":           b["Project"].(string) + ":" + b["Bucket"].(string),
					"Status":          "FAILURE",
					"UpdateTime":      spanner.CommitTimestamp,
					"BuildId":         int64(100),
					"OnCallRotations": []string{"rot"},
				})
				ms = append(ms, m)
			}
			_, err := span.Apply(ctx, ms)
			assert.Loosely(t, err, should.BeNil)
		}

		// Helper to verify if a builder exists in Spanner.
		assertBuilderExists := func(key string, exists bool) {
			row, err := span.ReadRow(span.Single(ctx), "BuilderStatuses", spanner.Key{key}, []string{"BuilderKey"})
			if exists {
				assert.Loosely(t, err, should.BeNil)
				var k string
				assert.Loosely(t, row.Column(0, &k), should.BeNil)
				assert.Loosely(t, k, should.Equal(key))
			} else {
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, errors.Is(err, spanner.ErrRowNotFound), should.BeTrue)
			}
		}

		t.Run("No builders in DB", func(t *ftt.Test) {
			err := CleanupStaleBuilders(ctx)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("All builders active", func(t *ftt.Test) {
			insertBuilders([]map[string]any{
				{"BuilderKey": "p1/b1/builder1", "Project": "p1", "Bucket": "b1", "Builder": "builder1"},
				{"BuilderKey": "p1/b1/builder2", "Project": "p1", "Bucket": "b1", "Builder": "builder2"},
			})

			// Mock ListBuilders to return both.
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), proto.MatcherEqual(&buildbucketpb.ListBuildersRequest{
				Project:  "p1",
				PageSize: 1000,
			})).Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{
					{Id: &buildbucketpb.BuilderID{Project: "p1", Bucket: "b1", Builder: "builder1"}},
					{Id: &buildbucketpb.BuilderID{Project: "p1", Bucket: "b1", Builder: "builder2"}},
				},
			}, nil)

			err := CleanupStaleBuilders(ctx)
			assert.Loosely(t, err, should.BeNil)

			assertBuilderExists("p1/b1/builder1", true)
			assertBuilderExists("p1/b1/builder2", true)
		})

		t.Run("Some builders stale", func(t *ftt.Test) {
			insertBuilders([]map[string]any{
				{"BuilderKey": "p1/b1/builder1", "Project": "p1", "Bucket": "b1", "Builder": "builder1"},
				{"BuilderKey": "p1/b1/builder2", "Project": "p1", "Bucket": "b1", "Builder": "builder2"},
			})

			// Mock ListBuilders to return only builder1.
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), proto.MatcherEqual(&buildbucketpb.ListBuildersRequest{
				Project:  "p1",
				PageSize: 1000,
			})).Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{
					{Id: &buildbucketpb.BuilderID{Project: "p1", Bucket: "b1", Builder: "builder1"}},
				},
			}, nil)

			err := CleanupStaleBuilders(ctx)
			assert.Loosely(t, err, should.BeNil)

			assertBuilderExists("p1/b1/builder1", true)
			assertBuilderExists("p1/b1/builder2", false) // Should be deleted
		})

		t.Run("Multiple projects", func(t *ftt.Test) {
			insertBuilders([]map[string]any{
				{"BuilderKey": "p1/b1/builder1", "Project": "p1", "Bucket": "b1", "Builder": "builder1"},
				{"BuilderKey": "p2/b1/builder1", "Project": "p2", "Bucket": "b1", "Builder": "builder1"},
			})

			// Mock ListBuilders for p1 (active)
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), proto.MatcherEqual(&buildbucketpb.ListBuildersRequest{
				Project:  "p1",
				PageSize: 1000,
			})).Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{
					{Id: &buildbucketpb.BuilderID{Project: "p1", Bucket: "b1", Builder: "builder1"}},
				},
			}, nil)

			// Mock ListBuilders for p2 (stale)
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), proto.MatcherEqual(&buildbucketpb.ListBuildersRequest{
				Project:  "p2",
				PageSize: 1000,
			})).Return(&buildbucketpb.ListBuildersResponse{
				Builders: []*buildbucketpb.BuilderItem{}, // empty, builder1 is stale
			}, nil)

			err := CleanupStaleBuilders(ctx)
			assert.Loosely(t, err, should.BeNil)

			assertBuilderExists("p1/b1/builder1", true)
			assertBuilderExists("p2/b1/builder1", false) // Should be deleted
		})

		t.Run("Permission denied on ListBuilders", func(t *ftt.Test) {
			insertBuilders([]map[string]any{
				{"BuilderKey": "p1/b1/builder1", "Project": "p1", "Bucket": "b1", "Builder": "builder1"},
			})

			// Mock ListBuilders to return Permission Denied.
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), proto.MatcherEqual(&buildbucketpb.ListBuildersRequest{
				Project:  "p1",
				PageSize: 1000,
			})).Return(nil, status.Error(codes.PermissionDenied, "denied"))

			err := CleanupStaleBuilders(ctx)
			assert.Loosely(t, err, should.NotBeNil)

			assertBuilderExists("p1/b1/builder1", true) // Should NOT be deleted
		})

		t.Run("Other error on ListBuilders", func(t *ftt.Test) {
			insertBuilders([]map[string]any{
				{"BuilderKey": "p1/b1/builder1", "Project": "p1", "Bucket": "b1", "Builder": "builder1"},
			})

			// Mock ListBuilders to return Internal error.
			mockBuildersClient.EXPECT().ListBuilders(gomock.Any(), proto.MatcherEqual(&buildbucketpb.ListBuildersRequest{
				Project:  "p1",
				PageSize: 1000,
			})).Return(nil, status.Error(codes.Internal, "internal error"))

			err := CleanupStaleBuilders(ctx)
			assert.Loosely(t, err, should.NotBeNil)

			assertBuilderExists("p1/b1/builder1", true) // Should NOT be deleted
		})
	})
}
