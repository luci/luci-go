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

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/commitingester/internal/taskspb"
	"go.chromium.org/luci/source_index/internal/config"
	"go.chromium.org/luci/source_index/internal/gitilesutil"
)

// RegisterCronHandlers registers the cron handlers for Source Index's commit
// ingestion.
func RegisterCronHandlers(srv *server.Server) error {
	cron.RegisterHandler("sync-commits", syncCommitsHandler)
	return nil
}

const workerCount = 8
const taskBatchSize = 1000

// syncCommitsHandler reads the config, scans gitiles for refs on all the
// configured repositories, and create commit-ingestion tasks to ensure commits
// on inactive refs are ingested.
//
// It scans the repositories in parallel. Failing to scan a repository will
// make it return an error, but does not cause it to skip unprocessed
// repositories.
func syncCommitsHandler(ctx context.Context) (err error) {
	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Fmt("get the config: %w", err)
	}

	taskC := make(chan *taskspb.IngestCommits, taskBatchSize)

	// Use parallel.RunMulti so that
	// 1. the repos can be processed in parallel, and
	// 2. more importantly, the error emerged when processing one repo does not
	//    block other repos from being processed.
	return parallel.RunMulti(ctx, workerCount, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(c chan<- func() error) {
			c <- func() error {
				err := scheduleTasks(ctx, taskC)
				return errors.WrapIf(err, "schedule tasks")
			}

			c <- func() error {
				defer close(taskC)
				return mr.RunMulti(func(c chan<- func() error) {
					for _, hostConfig := range cfg.HostConfigs() {
						c <- func() error {
							err := scanHostForTasks(ctx, mr, hostConfig, taskC)
							return errors.WrapIf(err, "scan host %q for tasks", hostConfig.Host())
						}
					}
				})
			}
		})
	})
}

// scanHostForTasks scans a gitiles host and create necessary commit ingestion
// tasks.
func scanHostForTasks(
	ctx context.Context,
	mr parallel.MultiRunner,
	hostCfg config.HostConfig,
	taskC chan<- *taskspb.IngestCommits,
) error {
	client, err := gitilesutil.NewClient(ctx, hostCfg.Host(), auth.AsSelf, auth.WithScopes(gitiles.OAuthScope))
	if err != nil {
		return errors.Fmt("initialize a Gitiles client: %w", err)
	}

	return mr.RunMulti(func(c chan<- func() error) {
		for _, repoConfig := range hostCfg.RepoConfigs() {
			for _, refPrefix := range config.IndexableRefPrefixes {
				c <- func() error {
					headsRes, err := client.Refs(ctx, &gitilespb.RefsRequest{Project: repoConfig.Name(), RefsPath: refPrefix})
					if err != nil {
						return errors.Fmt("query refs %q from repository %q: %w", refPrefix, repoConfig.Name(), err)
					}

					for ref := range headsRes.Revisions {
						if !repoConfig.ShouldIndexRef(ref) {
							continue
						}
						taskC <- &taskspb.IngestCommits{
							Host:       hostCfg.Host(),
							Repository: repoConfig.Name(),
							Commitish:  ref,
							PageToken:  "",
							TaskIndex:  0,
							// Schedule a commit ingestion task to the backfill queue so
							// creating a large amount of tasks in a short period of time
							// won't block the regular queue and trigger SLO alerts. Note that
							// most tasks are expected to be NOOP tasks.
							Backfill: true,
						}
					}
					return nil
				}
			}
		}
	})
}

// scheduleTasks schedule commit-ingestion tasks in batches.
func scheduleTasks(
	ctx context.Context,
	taskC <-chan *taskspb.IngestCommits,
) error {
	processBatch := func(currentBatch []*taskspb.IngestCommits) error {
		_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
			for _, task := range currentBatch {
				scheduleCommitIngestion(ctx, task)
			}
			return nil
		})
		return err
	}

	merr := make(errors.MultiError, 0)
	batch := make([]*taskspb.IngestCommits, 0, taskBatchSize)
	for task := range taskC {
		batch = append(batch, task)
		if len(batch) == taskBatchSize {
			currentBatch := batch
			batch = make([]*taskspb.IngestCommits, 0, taskBatchSize)
			if err := processBatch(currentBatch); err != nil {
				merr = append(merr, err)
			}
		}
	}

	// Process the remaining items.
	if err := processBatch(batch); err != nil {
		merr = append(merr, err)
	}

	return merr.AsError()
}
