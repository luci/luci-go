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

package revertculprit

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	lnpb "go.chromium.org/luci/luci_notify/api/service/v1"

	"go.chromium.org/luci/bisection/internal/lucinotify"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestNotifyRevertLanded(t *testing.T) {
	t.Parallel()

	ftt.Run("notifyRevertLanded", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)

		treeName := "chromium"
		revertURL := "https://chromium-review.googlesource.com/c/chromium/src/+/12346"

		culpritModel := &model.Suspect{
			Type:         model.SuspectType_NthSection,
			ReviewUrl:    "https://chromium-review.googlesource.com/c/chromium/src/+/12345",
			AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}

		t.Run("culprit is tree closer - notifies LUCI Notify", func(t *ftt.Test) {
			// Setup: Create analysis chain and suspect data
			_, _, analysis := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584903, "chromium", 444)
			analysis.IsTreeCloser = true
			assert.Loosely(t, datastore.Put(ctx, analysis), should.BeNil)

			// Create NthSectionAnalysis as the parent of the suspect
			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa), should.BeNil)

			culpritModel.ParentAnalysis = datastore.KeyForObj(ctx, nsa)
			culpritModel.GitilesCommit.Id = "abc123"
			assert.Loosely(t, datastore.Put(ctx, culpritModel), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Setup mock LUCI Notify client
			ctl := gomock.NewController(t)
			defer ctl.Finish()
			mockedClient := lucinotify.NewMockedClient(ctx, ctl)

			// Expect the RPC to be called
			mockedClient.Client.EXPECT().
				NotifyCulpritRevert(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, req *lnpb.NotifyCulpritRevertRequest, opts ...interface{}) (*emptypb.Empty, error) {
					// Verify request parameters
					assert.Loosely(t, req.TreeName, should.Equal(treeName))
					assert.Loosely(t, req.CulpritReviewUrl, should.Equal(culpritModel.ReviewUrl))
					assert.Loosely(t, req.RevertReviewUrl, should.Equal(revertURL))
					assert.Loosely(t, req.RevertLandTime, should.NotBeNil)
					return &emptypb.Empty{}, nil
				})

			// Execute with mocked context
			err := notifyRevertLanded(mockedClient.Ctx, treeName, culpritModel, revertURL)

			// Verify
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("culprit is NOT tree closer - skips notification", func(t *ftt.Test) {
			// Setup: Create analysis with IsTreeCloser = false
			_, _, analysis := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584904, "chromium", 445)
			analysis.IsTreeCloser = false
			assert.Loosely(t, datastore.Put(ctx, analysis), should.BeNil)

			// Create NthSectionAnalysis as the parent of the suspect
			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa), should.BeNil)

			culpritModel.ParentAnalysis = datastore.KeyForObj(ctx, nsa)
			culpritModel.GitilesCommit.Id = "def456"
			assert.Loosely(t, datastore.Put(ctx, culpritModel), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Setup mock - should NOT be called
			ctl := gomock.NewController(t)
			defer ctl.Finish()
			mockedClient := lucinotify.NewMockedClient(ctx, ctl)
			mockedClient.Client.EXPECT().NotifyCulpritRevert(gomock.Any(), gomock.Any()).Times(0)

			// Execute with mocked context
			err := notifyRevertLanded(mockedClient.Ctx, treeName, culpritModel, revertURL)

			// Verify - no error, but notification was skipped
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("error checking tree closer status - does not fail revert flow", func(t *ftt.Test) {
			// Setup: Culprit with invalid parent analysis reference
			invalidKey := datastore.NewKey(ctx, "CompileFailureAnalysis", "", 999999, nil)
			culpritModel.ParentAnalysis = invalidKey
			culpritModel.GitilesCommit.Id = "ghi789"

			// Execute
			err := notifyRevertLanded(ctx, treeName, culpritModel, revertURL)

			// Verify - should return nil (logs warning but doesn't fail)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("no suspects with same commit ID", func(t *ftt.Test) {
			// Setup: Culprit with unique commit ID
			culpritModel.GitilesCommit.Id = "unique-commit-xyz"
			culpritModel.ParentAnalysis = nil

			// Execute
			err := notifyRevertLanded(ctx, treeName, culpritModel, revertURL)

			// Verify - should return nil (not a tree closer, skips notification)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestIsCulpritTreeCloser(t *testing.T) {
	t.Parallel()

	ftt.Run("isCulpritTreeCloser", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)

		t.Run("single suspect with tree closer analysis", func(t *ftt.Test) {
			_, _, analysis := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584905, "chromium", 446)
			analysis.IsTreeCloser = true
			assert.Loosely(t, datastore.Put(ctx, analysis), should.BeNil)

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa), should.BeNil)

			culprit := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				AnalysisType:   pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			}
			culprit.GitilesCommit.Id = "commit123"
			assert.Loosely(t, datastore.Put(ctx, culprit), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			isTreeCloser, err := isCulpritTreeCloser(ctx, culprit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isTreeCloser, should.BeTrue)
		})

		t.Run("single suspect with non-tree-closer analysis", func(t *ftt.Test) {
			_, _, analysis := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584906, "chromium", 447)
			analysis.IsTreeCloser = false
			assert.Loosely(t, datastore.Put(ctx, analysis), should.BeNil)

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa), should.BeNil)

			culprit := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				AnalysisType:   pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			}
			culprit.GitilesCommit.Id = "commit456"
			assert.Loosely(t, datastore.Put(ctx, culprit), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			isTreeCloser, err := isCulpritTreeCloser(ctx, culprit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isTreeCloser, should.BeFalse)
		})

		t.Run("multiple suspects, at least one is tree closer", func(t *ftt.Test) {
			// Create two analyses
			_, _, analysis1 := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584907, "chromium", 448)
			analysis1.IsTreeCloser = false
			assert.Loosely(t, datastore.Put(ctx, analysis1), should.BeNil)

			_, _, analysis2 := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584908, "chromium", 449)
			analysis2.IsTreeCloser = true
			assert.Loosely(t, datastore.Put(ctx, analysis2), should.BeNil)

			nsa1 := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis1),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa1), should.BeNil)

			nsa2 := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis2),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa2), should.BeNil)

			// Create two suspects with the same commit ID
			commitID := "commit789"
			culprit1 := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				AnalysisType:   pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa1),
			}
			culprit1.GitilesCommit.Id = commitID
			culprit2 := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				AnalysisType:   pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa2),
			}
			culprit2.GitilesCommit.Id = commitID
			assert.Loosely(t, datastore.Put(ctx, culprit1, culprit2), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			isTreeCloser, err := isCulpritTreeCloser(ctx, culprit1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isTreeCloser, should.BeTrue)
		})

		t.Run("multiple suspects, none are tree closers", func(t *ftt.Test) {
			_, _, analysis1 := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584909, "chromium", 450)
			analysis1.IsTreeCloser = false
			assert.Loosely(t, datastore.Put(ctx, analysis1), should.BeNil)

			_, _, analysis2 := testutil.CreateCompileFailureAnalysisAnalysisChain(
				ctx, t, 88128398584910, "chromium", 451)
			analysis2.IsTreeCloser = false
			assert.Loosely(t, datastore.Put(ctx, analysis2), should.BeNil)

			nsa1 := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis1),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa1), should.BeNil)

			nsa2 := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(ctx, analysis2),
			}
			assert.Loosely(t, datastore.Put(ctx, nsa2), should.BeNil)

			commitID := "commit999"
			culprit1 := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				AnalysisType:   pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa1),
			}
			culprit1.GitilesCommit.Id = commitID
			culprit2 := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				AnalysisType:   pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa2),
			}
			culprit2.GitilesCommit.Id = commitID
			assert.Loosely(t, datastore.Put(ctx, culprit1, culprit2), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			isTreeCloser, err := isCulpritTreeCloser(ctx, culprit1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isTreeCloser, should.BeFalse)
		})

		t.Run("test failure suspect - ignored", func(t *ftt.Test) {
			culprit := &model.Suspect{
				Type:         model.SuspectType_NthSection,
				AnalysisType: pb.AnalysisType_TEST_FAILURE_ANALYSIS, // Not compile failure
			}
			culprit.GitilesCommit.Id = "test-commit"
			assert.Loosely(t, datastore.Put(ctx, culprit), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			isTreeCloser, err := isCulpritTreeCloser(ctx, culprit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isTreeCloser, should.BeFalse)
		})

		t.Run("suspect with nil ParentAnalysis", func(t *ftt.Test) {
			culprit := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				AnalysisType:   pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				ParentAnalysis: nil,
			}
			culprit.GitilesCommit.Id = "orphan-commit"
			assert.Loosely(t, datastore.Put(ctx, culprit), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			isTreeCloser, err := isCulpritTreeCloser(ctx, culprit)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isTreeCloser, should.BeFalse)
		})
	})
}
