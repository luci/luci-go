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
	"go.chromium.org/luci/source_index/internal/commitingester/internal/taskspb"
	"go.chromium.org/luci/source_index/internal/gitilesutil"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	commitIngestionTaskClass = "commit-ingestion"
	commitIngestionQueue     = "commit-ingestion"

	commitIngestionBackfillTaskClass = "commit-ingestion-backfill"
	commitIngestionBackfillQueue     = "commit-ingestion-backfill"
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

// commitIngestionBackfill is the same as commitIngestion, except that the task
// will be dispatched to the commit-ingestion-backfill task queue, which has
// lower rate limits and SLO target.
//
// This helps us ensure the normal commit-ingestion are prioritized and have a
// tighter SLO. Therefore the downstream services are less likely to run into
// data latency issue.
var commitIngestionBackfill = tq.RegisterTaskClass(tq.TaskClass{
	ID:        commitIngestionBackfillTaskClass,
	Prototype: &taskspb.IngestCommitsBackfill{},
	Queue:     commitIngestionBackfillQueue,
	Kind:      tq.Transactional,
})

// RegisterTaskQueueHandlers registers the task queue handlers for Source
// Index's commit ingestion.
func RegisterTaskQueueHandlers(srv *server.Server) error {
	commitIngestion.AttachHandler(handleCommitIngestion)
	commitIngestionBackfill.AttachHandler(handleCommitIngestionBackfill)

	return nil
}

// scheduleCommitIngestion enqueues a task to ingest commits from a Gitiles
// repository starting from the specified commitish.
func scheduleCommitIngestion(ctx context.Context, task *taskspb.IngestCommits, isBackfill bool) {
	var payload proto.Message = task
	if isBackfill {
		// Re-create the task using `taskspb.IngestCommitsBackfill` here so the
		// caller doesn't need to know the difference between
		// `taskspb.IngestCommitsBackfill` and `taskspb.IngestCommits`.
		//
		// TODO: make the tq package support dispatching tasks with an explicit
		// task queue name so we don't need to have multiple payload types.
		payload = &taskspb.IngestCommitsBackfill{
			Host:       task.Host,
			Repository: task.Repository,
			Commitish:  task.Commitish,
			PageToken:  task.PageToken,
			TaskIndex:  task.TaskIndex,
		}
	}

	tq.MustAddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-%s-page-%d", task.Host, task.Repository, task.Commitish, task.TaskIndex),
		Payload: payload,
	})
}

func handleCommitIngestion(ctx context.Context, payload proto.Message) error {
	task := payload.(*taskspb.IngestCommits)
	return processCommitIngestionTask(ctx, task, false)
}

func handleCommitIngestionBackfill(ctx context.Context, payload proto.Message) error {
	task := payload.(*taskspb.IngestCommitsBackfill)
	return processCommitIngestionTask(ctx, &taskspb.IngestCommits{
		Host:       task.Host,
		Repository: task.Repository,
		Commitish:  task.Commitish,
		PageToken:  task.PageToken,
		TaskIndex:  task.TaskIndex,
	}, true)
}

const (
	// firstTaskPageSize is the page size used for the first commit ingestion task
	// (a commit ingestion task with empty page token).
	//
	// Use a smaller page size for the first page to reduce the amount of
	// commits we need to query from Gitiles. Typically,
	// * only a few commits need to be ingested if the task was scheduled by
	//   the gitiles pubsub handler.
	// * on most refs, no commit need to be ingested if the task was scheduled
	//   by the sync-commits cron job.
	firstTaskPageSize   = 100
	regularTaskPageSize = 1000
)

func processCommitIngestionTask(ctx context.Context, task *taskspb.IngestCommits, isBackfill bool) error {
	ctx = logging.SetField(ctx, "host", task.Host)
	ctx = logging.SetField(ctx, "repository", task.Repository)
	ctx = logging.SetField(ctx, "commitish", task.Commitish)
	ctx = logging.SetField(ctx, "task_index", task.TaskIndex)

	logging.Infof(ctx, "received commit ingestion task with page token: %q", task.PageToken)

	client, err := gitilesutil.NewClient(ctx, task.Host, auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	if err != nil {
		return errors.Annotate(err, "initialize a Gitiles client").Err()
	}

	var pageSize int32 = regularTaskPageSize
	if task.PageToken == "" {
		pageSize = firstTaskPageSize
	}
	req := &gitilespb.LogRequest{
		Project:    task.Repository,
		Committish: task.Commitish,
		PageToken:  task.PageToken,
		PageSize:   pageSize,
	}
	res, err := client.Log(ctx, req)
	if err != nil {
		return errors.Annotate(grpcutil.WrapIfTransient(err), "query Gitiles logs").Err()
	}

	shouldIngestNextPage := res.NextPageToken != ""

	// If the first commit has already been ingested, all of its ancestor commits
	// should've been (or scheduled to be) ingested. Stop ingesting.  This is
	// likely to happen when we are syncing the start of a ref.
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
		// the transaction to write commits are blind writes.
		// The Commits table only contain derived data (other than the update
		// timestamp). It's OK to overwrite the rows.
		exists, err := commit.Exists(span.Single(ctx), key)
		if err != nil {
			return errors.Annotate(err, "check whether the first commit in the page was already ingested").Err()
		}
		if exists {
			logging.Infof(ctx, "commit %q is already ingested; stop ingesting", res.Log[0].Id)
			return nil
		}
	}

	// If the last commit has already been ingested, all of its ancestor commits
	// should've been (or scheduled to be) ingested. We do not need to schedule
	// a continuation task.
	if len(res.Log) > 0 && shouldIngestNextPage {
		key, err := commit.NewKey(task.Host, task.Repository, res.Log[len(res.Log)-1].Id)
		if err != nil {
			return errors.Annotate(err, "construct commit key for the last commit in the page").Err()
		}
		// Keep the Exists check outside of the transaction to write the commits so
		// the transaction to write commits are blind writes.
		exists, err := commit.Exists(span.Single(ctx), key)
		if err != nil {
			return errors.Annotate(err, "check whether the last commit in the page was already ingested").Err()
		}
		shouldIngestNextPage = !exists
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
		if shouldIngestNextPage {
			scheduleCommitIngestion(
				ctx,
				&taskspb.IngestCommits{
					Host:       task.Host,
					Repository: task.Repository,
					Commitish:  task.Commitish,
					TaskIndex:  task.TaskIndex + 1,
					PageToken:  res.NextPageToken,
				},
				// Schedule the continuation task to the same task queue.
				// In case a pubsub created task needs a continuation task (e.g. fast
				// forward merge from a feature branch), we don't want the continuation
				// task to be blocked by cron-created backfill tasks.
				isBackfill,
			)
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "saving commits").Err()
	}

	logging.Infof(ctx, "finished commit ingestion task")
	return nil
}
