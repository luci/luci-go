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

package commitingester

import (
	"context"
	"fmt"
	"net/http"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/source_index/internal/commit"
	"go.chromium.org/luci/source_index/internal/commitingester/taskspb"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	commitIngestionTaskClass = "commit-ingestion"
	commitIngestionQueue     = "commit-ingestion"
)

var commitIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        commitIngestionTaskClass,
	Prototype: &taskspb.IngestCommits{},
	Queue:     commitIngestionQueue,
	// Use `tq.Transactional` instead of `tq.FollowsContext` so `tq.MustAddTask`
	// either always panics when not in a transaction context, or never panics
	// when in a transaction context (instead of occasionally panic when it failed
	// to schedule a task when not used in a transaction context).
	Kind: tq.Transactional,
})

// RegisterTaskQueueHandlers registers the task queue handlers for Source
// Index's commit ingestion.
func RegisterTaskQueueHandlers(srv *server.Server) error {
	commitIngestion.AttachHandler(handleCommitIngestion)

	return nil
}

// scheduleCommitIngestion enqueues a task to ingest commits from a Gitiles
// repository starting from the specified commitish.
func scheduleCommitIngestion(ctx context.Context, task *taskspb.IngestCommits) {
	tq.MustAddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-%s-page-%d", task.Host, task.Repository, task.Commitish, task.TaskIndex),
		Payload: task,
	})
}

func handleCommitIngestion(ctx context.Context, payload proto.Message) error {
	task := payload.(*taskspb.IngestCommits)

	ctx = logging.SetField(ctx, "host", task.Host)
	ctx = logging.SetField(ctx, "repository", task.Repository)
	ctx = logging.SetField(ctx, "commitish", task.Commitish)
	ctx = logging.SetField(ctx, "task_index", task.TaskIndex)

	logging.Infof(ctx, "received commit ingestion task with page token: %q", task.PageToken)

	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	if err != nil {
		return err
	}
	client, err := gitiles.NewRESTClient(&http.Client{Transport: t}, task.Host, false)
	if err != nil {
		return errors.Annotate(err, "initialize a Gitiles REST client").Err()
	}

	return processCommitIngestionTask(ctx, client, task)
}

func processCommitIngestionTask(ctx context.Context, client gitilespb.GitilesClient, task *taskspb.IngestCommits) error {
	req := &gitilespb.LogRequest{
		Project:    task.Repository,
		Committish: task.Commitish,
		PageToken:  task.PageToken,
		PageSize:   1000,
	}
	res, err := client.Log(ctx, req)
	if err != nil {
		return errors.Annotate(grpcutil.WrapIfTransient(err), "query Gitiles logs").Err()
	}

	// If the first commit has already been ingested, all of its ancestor commits
	// should've been (or scheduled to be) ingested. Stop ingesting.
	//
	// N.B. we cannot use `task.Commitish` to check the existence of the commit.
	// `task.Commitish` may not be a hash. And the first commit may not be the
	// commitish due to page token.
	if len(res.Log) > 0 {
		key, err := commit.NewKey(task.Host, task.Repository, res.Log[0].Id)
		if err != nil {
			return errors.Annotate(err, "construct commit key for the first commit in the page").Err()
		}

		// Keep the Exists check outside of the transaction to write the commits so
		// 1. the transaction to write commits are blind writes; and
		// 2. checking the existence of a commit only requires a read transaction.
		//    After the initial ingestion, we expect half of the tasks to be
		//	  ignored. Only the first ingestion task kicked of by a pubsub event
		//    result in an update. All the subsequent tasks will likely be ignored;
		//    and
		// 3. the Commits table only contain derived data (other than update
		//    timestamp). It's OK to overwrite the rows.
		exists, err := commit.Exists(span.Single(ctx), key)
		if err != nil {
			return errors.Annotate(err, "check whether the commit was already ingested").Err()
		}
		if exists {
			logging.Infof(ctx, "commit %q is already ingested; stop ingesting", res.Log[0].Id)
			return nil
		}
	}

	// Prepare commits to write to the database.
	var commitsToSave = make([]*spanner.Mutation, 0, len(res.Log))
	for _, log := range res.Log {
		gitCommit, err := commit.NewGitCommit(task.Host, task.Repository, log)
		if err != nil {
			return errors.Annotate(err, "converting git commit to source-index's representation").Err()
		}

		_, err = gitCommit.Position()
		if err != nil {
			// We don't want a commit with a malformed footer to break our ingestion
			// pipeline. Log the error and move on.
			logging.WithError(err).Warningf(ctx, "unable to extract position from commit %s", gitCommit.Key().URL())
		}

		commitsToSave = append(commitsToSave, commit.NewFromGitCommit(gitCommit).Save())
	}

	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		span.BufferWrite(ctx, commitsToSave...)
		if res.NextPageToken != "" {
			scheduleCommitIngestion(ctx, &taskspb.IngestCommits{
				Host:       task.Host,
				Repository: task.Repository,
				Commitish:  task.Commitish,
				TaskIndex:  task.TaskIndex + 1,
				PageToken:  res.NextPageToken,
			})
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "saving commits").Err()
	}

	logging.Infof(ctx, "finished commit ingestion task")
	return nil
}
