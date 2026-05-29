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
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/luci_notify/internal/testutil"
)

func TestMain(m *testing.M) {
	testutil.SpannerTestMain(m)
}

func TestTrackBuilderStatus(t *testing.T) {
	ftt.Run("trackBuilderStatus", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		builderID := &buildbucketpb.BuilderID{
			Project: "chromium",
			Bucket:  "ci",
			Builder: "test-builder",
		}

		t.Run("Insert new builder status", func(t *ftt.Test) {
			build := &Build{
				Build: &buildbucketpb.Build{
					Id:      100,
					Builder: builderID,
					Status:  buildbucketpb.Status_FAILURE,
					Input: &buildbucketpb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"gardener_rotations": structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
									structpb.NewStringValue("angle"),
								}}),
							},
						},
					},
				},
			}

			err := trackBuilderStatus(ctx, build)
			assert.Loosely(t, err, should.BeNil)

			// Verify row in Spanner.
			var status string
			var buildId int64
			var rotations []string
			builderKey := "chromium/ci/test-builder"
			row, err := span.ReadRow(span.Single(ctx), "BuilderStatuses", spanner.Key{builderKey}, []string{"Status", "BuildId", "OnCallRotations"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, row.Column(0, &status), should.BeNil)
			assert.Loosely(t, row.Column(1, &buildId), should.BeNil)
			assert.Loosely(t, row.Column(2, &rotations), should.BeNil)

			assert.Loosely(t, status, should.Equal("FAILURE"))
			assert.Loosely(t, buildId, should.Equal(100))
			assert.Loosely(t, rotations, should.Match([]string{"angle"}))
		})

		t.Run("Update with newer build (smaller ID)", func(t *ftt.Test) {
			// Pre-insert a row.
			builderKey := "chromium/ci/test-builder"
			m := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
				"BuilderKey":      builderKey,
				"Project":         "chromium",
				"Bucket":          "ci",
				"Builder":         "test-builder",
				"Realm":           "chromium:ci",
				"Status":          "FAILURE",
				"UpdateTime":      spanner.CommitTimestamp,
				"BuildId":         int64(100),
				"OnCallRotations": []string{"angle"},
			})
			_, err := span.Apply(ctx, []*spanner.Mutation{m})
			assert.Loosely(t, err, should.BeNil)

			// New build with smaller ID (newer).
			build := &Build{
				Build: &buildbucketpb.Build{
					Id:      50,
					Builder: builderID,
					Status:  buildbucketpb.Status_SUCCESS,
				},
			}

			err = trackBuilderStatus(ctx, build)
			assert.Loosely(t, err, should.BeNil)

			// Verify row updated.
			var status string
			var buildId int64
			row, err := span.ReadRow(span.Single(ctx), "BuilderStatuses", spanner.Key{builderKey}, []string{"Status", "BuildId"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, row.Column(0, &status), should.BeNil)
			assert.Loosely(t, row.Column(1, &buildId), should.BeNil)
			assert.Loosely(t, status, should.Equal("SUCCESS"))
			assert.Loosely(t, buildId, should.Equal(50))
		})

		t.Run("Do not update with older build (larger ID)", func(t *ftt.Test) {
			// Pre-insert a row.
			builderKey := "chromium/ci/test-builder"
			m := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
				"BuilderKey":      builderKey,
				"Project":         "chromium",
				"Bucket":          "ci",
				"Builder":         "test-builder",
				"Realm":           "chromium:ci",
				"Status":          "FAILURE",
				"UpdateTime":      spanner.CommitTimestamp,
				"BuildId":         int64(100),
				"OnCallRotations": []string{"angle"},
			})
			_, err := span.Apply(ctx, []*spanner.Mutation{m})
			assert.Loosely(t, err, should.BeNil)

			// New build with larger ID (older).
			build := &Build{
				Build: &buildbucketpb.Build{
					Id:      150,
					Builder: builderID,
					Status:  buildbucketpb.Status_SUCCESS,
				},
			}

			err = trackBuilderStatus(ctx, build)
			assert.Loosely(t, err, should.BeNil)

			// Verify row NOT updated.
			var status string
			var buildId int64
			row, err := span.ReadRow(span.Single(ctx), "BuilderStatuses", spanner.Key{builderKey}, []string{"Status", "BuildId"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, row.Column(0, &status), should.BeNil)
			assert.Loosely(t, row.Column(1, &buildId), should.BeNil)
			assert.Loosely(t, status, should.Equal("FAILURE"))
			assert.Loosely(t, buildId, should.Equal(100))
		})

		t.Run("Do not update with non-final status", func(t *ftt.Test) {
			// Pre-insert a row.
			builderKey := "chromium/ci/test-builder"
			m := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
				"BuilderKey":      builderKey,
				"Project":         "chromium",
				"Bucket":          "ci",
				"Builder":         "test-builder",
				"Realm":           "chromium:ci",
				"Status":          "FAILURE",
				"UpdateTime":      spanner.CommitTimestamp,
				"BuildId":         int64(100),
				"OnCallRotations": []string{"angle"},
			})
			_, err := span.Apply(ctx, []*spanner.Mutation{m})
			assert.Loosely(t, err, should.BeNil)

			// New build with smaller ID (newer) but non-final status.
			build := &Build{
				Build: &buildbucketpb.Build{
					Id:      50,
					Builder: builderID,
					Status:  buildbucketpb.Status_STARTED,
				},
			}

			err = trackBuilderStatus(ctx, build)
			assert.Loosely(t, err, should.BeNil)

			// Verify row NOT updated.
			var status string
			var buildId int64
			row, err := span.ReadRow(span.Single(ctx), "BuilderStatuses", spanner.Key{builderKey}, []string{"Status", "BuildId"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, row.Column(0, &status), should.BeNil)
			assert.Loosely(t, row.Column(1, &buildId), should.BeNil)
			assert.Loosely(t, status, should.Equal("FAILURE"))
			assert.Loosely(t, buildId, should.Equal(100))
		})
	})
}
