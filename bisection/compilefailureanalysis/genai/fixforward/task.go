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

package fixforward

import (
	"context"

	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/llm"
	"go.chromium.org/luci/bisection/model"
	taskpb "go.chromium.org/luci/bisection/task/proto"
)

var GenerateFixforwardTasks = tq.RegisterTaskClass(tq.TaskClass{
	ID:        "generate-fixforward-action",
	Prototype: (*taskpb.GenerateFixforwardTask)(nil),
	Queue:     "generate-fixforward-action",
	Kind:      tq.NonTransactional,
})

// RegisterTaskClass registers the task class for tq dispatcher
func RegisterTaskClass(srv *server.Server) {
	GenerateFixforwardTasks.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskpb.GenerateFixforwardTask)

		// In a real execution, we'd initialize clients from server context config.
		// For now we initialize them ad-hoc to fulfill the task.
		genaiClient, err := llm.NewClient(ctx, srv.Options.CloudProject)
		if err != nil {
			return errors.Annotate(err, "failed creating LLM client").Err()
		}

		gerritClient, err := gerrit.NewClient(ctx, "chromium-review.googlesource.com")
		if err != nil {
			return errors.Annotate(err, "failed creating Gerrit client").Err()
		}

		return processGenerateFixforwardTask(ctx, task, genaiClient, gerritClient)
	})
}

func processGenerateFixforwardTask(ctx context.Context, task *taskpb.GenerateFixforwardTask, genaiClient llm.Client, gerritClient *gerrit.Client) error {
	analysisID := task.GetAnalysisId()
	culpritID := task.GetCulpritId()

	cfa := &model.CompileFailureAnalysis{Id: analysisID}
	if err := datastore.Get(ctx, cfa); err != nil {
		return errors.Annotate(err, "failed getting CompileFailureAnalysis").Err()
	}

	suspect := &model.Suspect{
		Id:             culpritID,
		ParentAnalysis: datastore.KeyForObj(ctx, cfa),
	}
	if err := datastore.Get(ctx, suspect); err != nil {
		return errors.Annotate(err, "failed getting Suspect").Err()
	}

	// We'd need failure log here. Just passing empty for testing.
	// We'd also need the repoUrl.
	// For testing, we mock these out.
	var commit *buildbucketpb.GitilesCommit
	if suspect.GitilesCommit.GetId() != "" {
		commit = &suspect.GitilesCommit
	}
	repoUrl := gitiles.GetRepoUrl(ctx, commit) // Simplified

	logging.Infof(ctx, "Executing GenerateFixforwardTask for culprit %s", suspect.GitilesCommit.GetId())
	err := GenerateFixforwardCL(ctx, genaiClient, gerritClient, cfa, suspect.GitilesCommit.GetId(), "Fake failure log", repoUrl, suspect.RevertURL)
	if err != nil {
		logging.Errorf(ctx, "GenerateFixforwardCL failed: %v", err)
		return err
	}

	return nil
}
