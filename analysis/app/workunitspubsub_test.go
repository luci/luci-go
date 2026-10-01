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

package app

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/workunits/exporter"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
)

func TestWorkUnitsPubSubHandler(t *testing.T) {
	ftt.Run("WorkUnitsPubSubHandler", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		fakeClient := exporter.NewFakeClient()
		wuExporter := exporter.NewExporter(fakeClient)
		h := NewWorkUnitsPubSubHandler(wuExporter)

		t.Run("Handle", func(t *ftt.Test) {
			notification := &rdbpb.WorkUnitsNotification{
				WorkUnits: []*rdbpb.WorkUnitsNotification_WorkUnitDetails{
					{
						WorkUnitName: "rootInvocations/u-root-inv/workUnits/wu-1",
						HasArtifacts: true,
						MergedInheritedProperties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"key": structpb.NewStringValue("value"),
							},
						},
						WorkUnit: &rdbpb.WorkUnit{
							Name:       "rootInvocations/u-root-inv/workUnits/wu-1",
							WorkUnitId: "wu-1",
							Parent:     "rootInvocations/u-root-inv/workUnits/wu-parent",
							Kind:       "TF_MODULE",
							State:      rdbpb.WorkUnit_SUCCEEDED,
						},
					},
					{
						WorkUnitName: "rootInvocations/u-root-inv/workUnits/wu-root",
						WorkUnit: &rdbpb.WorkUnit{
							Name:       "rootInvocations/u-root-inv/workUnits/wu-root",
							WorkUnitId: "wu-root",
							Parent:     "invocations/u-root-inv", // Root Invocation
							Kind:       "TF_MODULE",
							State:      rdbpb.WorkUnit_SUCCEEDED,
						},
					},
				},
				ResultdbHost: "results.api.cr.dev",
				RootInvocationMetadata: &rdbpb.RootInvocationMetadata{
					Realm:            "test-project:try",
					RootInvocationId: "u-root-inv",
				},
				DeduplicationKey: "dedup-key-123",
			}
			message := pubsub.Message{
				Attributes: map[string]string{
					"luci_project": "test-project",
				},
			}

			err := h.Handle(ctx, message, notification)
			assert.NoErr(t, err)

			insertions := fakeClient.InsertionsByDestinationKey[exporter.WorkUnitTable.Key]
			assert.Loosely(t, len(insertions), should.Equal(2))

			// Verify first work unit
			row1 := insertions[0]
			assert.Loosely(t, row1.Project, should.Equal("test-project"))
			assert.Loosely(t, row1.RootInvocationId, should.Equal("u-root-inv"))
			assert.Loosely(t, row1.WorkUnitId, should.Equal("wu-1"))
			assert.Loosely(t, row1.ParentWorkUnit, should.Equal("rootInvocations/u-root-inv/workUnits/wu-parent"))
			assert.Loosely(t, row1.MergedInheritedProperties, should.Equal(`{"key":"value"}`))
			assert.Loosely(t, row1.Kind, should.Equal("TF_MODULE"))
			assert.Loosely(t, row1.State, should.Equal(rdbpb.WorkUnit_SUCCEEDED))

			// Verify second work unit (root)
			row2 := insertions[1]
			assert.Loosely(t, row2.Project, should.Equal("test-project"))
			assert.Loosely(t, row2.RootInvocationId, should.Equal("u-root-inv"))
			assert.Loosely(t, row2.WorkUnitId, should.Equal("wu-root"))
			assert.Loosely(t, row2.ParentWorkUnit, should.BeEmpty) // Should be empty because parent was Root Invocation
			assert.Loosely(t, row2.Kind, should.Equal("TF_MODULE"))
			assert.Loosely(t, row2.State, should.Equal(rdbpb.WorkUnit_SUCCEEDED))

			// Verify checkpoint was recorded in Spanner.
			cps, err := checkpoints.ReadAllForTesting(span.Single(ctx))
			assert.NoErr(t, err)
			assert.Loosely(t, cps, should.HaveLength(1))
			assert.Loosely(t, cps[0].Key, should.Match(checkpoints.Key{
				Project:    "test-project",
				ResourceID: "results.api.cr.dev/u-root-inv",
				ProcessID:  "work-unit-ingestion/export-work-units/work-units",
				Uniquifier: "dedup-key-123",
			}))

			t.Run("Work units are not exported again if duplicate message is received", func(t *ftt.Test) {
				fakeClient.InsertionsByDestinationKey = map[string][]*bqpb.WorkUnitRow{}

				err := h.Handle(ctx, message, notification)
				assert.NoErr(t, err)

				// Nothing should be exported because the checkpoint already exists.
				assert.Loosely(t, fakeClient.InsertionsByDestinationKey, should.BeEmpty)
			})
		})

		t.Run("Missing project", func(t *ftt.Test) {
			notification := &rdbpb.WorkUnitsNotification{
				RootInvocationMetadata: &rdbpb.RootInvocationMetadata{
					// Missing Realm
					RootInvocationId: "u-root-inv",
				},
			}
			message := pubsub.Message{}

			err := h.Handle(ctx, message, notification)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, pubsub.Ignore.In(err), should.BeTrue)
		})
	})
}
