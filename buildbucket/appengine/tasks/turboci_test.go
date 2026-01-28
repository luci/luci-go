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

package tasks

import (
	"context"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/turboci/data"
	stagepb "go.chromium.org/turboci/proto/go/data/stage/v1"

	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestWriteStageAttemptOnBuildCompletion(t *testing.T) {
	t.Parallel()

	ftt.Run("writeStageAttemptOnBuildCompletion", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		mockOrch := &turboci.FakeOrchestratorClient{}
		ctx = turboci.WithTurboCIOrchestratorClient(ctx, mockOrch)

		t.Run("not found", func(t *ftt.Test) {
			assert.ErrIsLike(t, writeStageAttemptOnBuildCompletion(ctx, 1, pb.Status_STARTED), "build 1 not found")
		})

		t.Run("build still active", func(t *ftt.Test) {
			bld := &model.Build{
				ID: 2,
				Proto: &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_STARTED,
				},
			}
			assert.NoErr(t, datastore.Put(ctx, bld))
			assert.ErrIsLike(t, writeStageAttemptOnBuildCompletion(ctx, 2, pb.Status_STARTED), "build 2 has not ended yet, current status: STARTED")
		})

		t.Run("build doesn't have a stage attempt token", func(t *ftt.Test) {
			bld := &model.Build{
				ID: 3,
				Proto: &pb.Build{
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
			}
			assert.NoErr(t, datastore.Put(ctx, bld))
			assert.ErrIsLike(t, writeStageAttemptOnBuildCompletion(ctx, 3, pb.Status_STARTED), "build 3 doesn't have a stage attempt token, cannot update TurboCI")
		})

		buildToUpdate := func(bID int, status pb.Status) *model.Build {
			bld := &model.Build{
				ID:                int64(bID),
				StageAttemptID:    "Lplan-id:Sstage-id:A1",
				StageAttemptToken: "token",
				Proto: &pb.Build{
					Id: int64(bID),
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: status,
				},
			}
			assert.NoErr(t, datastore.Put(ctx, bld))
			return bld
		}

		t.Run("success", func(t *ftt.Test) {
			bld := buildToUpdate(4, pb.Status_SUCCESS)
			assert.NoErr(t, writeStageAttemptOnBuildCompletion(ctx, bld.ID, pb.Status_STARTED))
			assert.Loosely(t, mockOrch.LastWriteNodesCall, should.NotBeNil)
			cw := mockOrch.LastWriteNodesCall.GetCurrentAttempt()
			assert.That(t, cw.GetStateTransition().HasComplete(), should.BeTrue)
		})

		t.Run("canceled", func(t *ftt.Test) {
			bld := buildToUpdate(5, pb.Status_CANCELED)
			assert.NoErr(t, writeStageAttemptOnBuildCompletion(ctx, bld.ID, pb.Status_STARTED))
			assert.Loosely(t, mockOrch.LastWriteNodesCall, should.NotBeNil)
			cw := mockOrch.LastWriteNodesCall.GetCurrentAttempt()
			assert.That(t, cw.GetStateTransition().HasIncomplete(), should.BeTrue)
			process := cw.GetProgress()
			assert.That(t, len(process), should.Equal(1))
			assert.That(t, process[0].GetMessage(), should.Equal("Cancelled via Buildbucket"))
		})

		t.Run("infra failure", func(t *ftt.Test) {
			bld := buildToUpdate(6, pb.Status_INFRA_FAILURE)
			bld.Proto.StatusDetails = &pb.StatusDetails{
				ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
				Timeout:            &pb.StatusDetails_Timeout{},
			}
			assert.NoErr(t, datastore.Put(ctx, bld))
			assert.NoErr(t, writeStageAttemptOnBuildCompletion(ctx, bld.ID, pb.Status_STARTED))
			assert.Loosely(t, mockOrch.LastWriteNodesCall, should.NotBeNil)
			cw := mockOrch.LastWriteNodesCall.GetCurrentAttempt()
			assert.That(t, cw.GetStateTransition().HasIncomplete(), should.BeTrue)
			process := cw.GetProgress()
			assert.That(t, len(process), should.Equal(1))
			assert.That(t, process[0].GetMessage(), should.Equal("Resource exhausted\nTimed out"))
			assert.That(t, data.ExtractValue[*pb.StatusDetails](process[0].GetDetails()[0]), should.Match(bld.Proto.StatusDetails))
		})

		t.Run("build didn't start", func(t *ftt.Test) {
			t.Run("cancel attempt with build details", func(t *ftt.Test) {
				bld := buildToUpdate(7, pb.Status_CANCELED)
				infra := &model.BuildInfra{
					Build: datastore.KeyForObj(ctx, bld),
					Proto: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
					},
				}
				assert.NoErr(t, datastore.Put(ctx, infra))
				assert.NoErr(t, writeStageAttemptOnBuildCompletion(ctx, bld.ID, pb.Status_SCHEDULED))
				assert.Loosely(t, mockOrch.LastWriteNodesCall, should.NotBeNil)
				cw := mockOrch.LastWriteNodesCall.GetCurrentAttempt()
				assert.That(t, cw.GetStateTransition().HasIncomplete(), should.BeTrue)
				process := cw.GetProgress()
				assert.That(t, len(process), should.Equal(1))
				assert.That(t, process[0].GetMessage(), should.Equal("Cancelled via Buildbucket"))
				details := cw.GetDetails()
				assert.That(t, len(details), should.Equal(2))
				bldDetails := &pb.BuildStageDetails{}
				assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[0].GetValue().UnmarshalTo(bldDetails))
				assert.That(t, bldDetails.GetId(), should.Equal(bld.ID))
				commonDetails := &stagepb.CommonStageAttemptDetails{}
				assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[1].GetValue().UnmarshalTo(commonDetails))
				assert.That(t, commonDetails.GetViewUrls()["Buildbucket"].GetUrl(), should.Equal(fmt.Sprintf("https://app.appspot.com/build/%d", bld.ID)))
			})
			t.Run("fail attempt with build details", func(t *ftt.Test) {
				bld := buildToUpdate(8, pb.Status_INFRA_FAILURE)
				bld.Proto.StatusDetails = &pb.StatusDetails{
					ResourceExhaustion: &pb.StatusDetails_ResourceExhaustion{},
					Timeout:            &pb.StatusDetails_Timeout{},
				}
				infra := &model.BuildInfra{
					Build: datastore.KeyForObj(ctx, bld),
					Proto: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "app.appspot.com",
						},
					},
				}
				assert.NoErr(t, datastore.Put(ctx, bld, infra))
				assert.NoErr(t, writeStageAttemptOnBuildCompletion(ctx, bld.ID, pb.Status_SCHEDULED))
				assert.Loosely(t, mockOrch.LastWriteNodesCall, should.NotBeNil)
				cw := mockOrch.LastWriteNodesCall.GetCurrentAttempt()
				assert.That(t, cw.GetStateTransition().HasIncomplete(), should.BeTrue)
				process := cw.GetProgress()
				assert.That(t, len(process), should.Equal(1))
				assert.That(t, process[0].GetMessage(), should.Equal("Resource exhausted\nTimed out"))
				assert.That(t, data.ExtractValue[*pb.StatusDetails](process[0].GetDetails()[0]), should.Match(bld.Proto.StatusDetails))
				details := cw.GetDetails()
				assert.That(t, len(details), should.Equal(2))
				bldDetails := &pb.BuildStageDetails{}
				assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[0].GetValue().UnmarshalTo(bldDetails))
				assert.That(t, bldDetails.GetId(), should.Equal(bld.ID))
				commonDetails := &stagepb.CommonStageAttemptDetails{}
				assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[1].GetValue().UnmarshalTo(commonDetails))
				assert.That(t, commonDetails.GetViewUrls()["Buildbucket"].GetUrl(), should.Equal(fmt.Sprintf("https://app.appspot.com/build/%d", bld.ID)))
			})
		})
	})
}
