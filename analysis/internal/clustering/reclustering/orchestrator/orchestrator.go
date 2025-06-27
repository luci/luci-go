// Copyright 2022 The LUCI Authors.
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

package orchestrator

import (
	"context"
	"math/big"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	worker "go.chromium.org/luci/analysis/internal/clustering/reclustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/clustering/shards"
	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/services/reclustering"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
)

var (
	chunkGauge = metric.NewInt("analysis/clustering/reclustering/chunk_count",
		"The estimated number of chunks, by LUCI Project.",
		&types.MetricMetadata{
			Units: "chunks",
		},
		// The LUCI project.
		field.String("project"),
	)

	workersGauge = metric.NewInt("analysis/clustering/reclustering/worker_count",
		"The number of workers performing reclustering, by LUCI Project.",
		&types.MetricMetadata{
			Units: "workers",
		},
		// The LUCI project.
		field.String("project"),
	)

	progressGauge = metric.NewFloat("analysis/clustering/reclustering/progress",
		"The progress re-clustering, measured from 0 to 1, by LUCI Project.",
		&types.MetricMetadata{
			Units: "completions",
		},
		// The LUCI project.
		field.String("project"),
	)

	lastCompletedGauge = metric.NewInt("analysis/clustering/reclustering/last_completed",
		"UNIX timstamp of the last completed re-clustering, by LUCI Project. "+
			"Not reported until at least one re-clustering completes.",
		&types.MetricMetadata{
			Units: types.Seconds,
		},
		// The LUCI project.
		field.String("project"),
	)

	// statusGauge reports the status of the orchestrator run. This covers
	// only the orchestrator and not the success/failure of workers.
	// Valid values are: "disabled", "success", "failure".
	statusGauge = metric.NewString("analysis/clustering/reclustering/orchestrator_status",
		"Whether the orchestrator is enabled and succeeding.",
		nil)
)

// CronHandler is the entry-point to the orchestrator that creates
// reclustering jobs. It is triggered by a cron job configured in
// cron.yaml.
func CronHandler(ctx context.Context) error {
	err := orchestrate(ctx)
	if err != nil {
		logging.Errorf(ctx, "Reclustering orchestrator encountered errors: %s", err)
		return err
	}
	return nil
}

func init() {
	// Register metrics as global metrics, which has the effort of
	// resetting them after every flush.
	tsmon.RegisterGlobalCallback(func(ctx context.Context) {
		// Do nothing -- the metrics will be populated by the cron
		// job itself and does not need to be triggered externally.
	}, chunkGauge, workersGauge, progressGauge, lastCompletedGauge, statusGauge)
}

func orchestrate(ctx context.Context) error {
	status := "failure"
	defer func() {
		// Closure for late binding.
		statusGauge.Set(ctx, status)
	}()

	projects, err := state.ReadProjects(span.Single(ctx))
	if err != nil {
		return errors.Fmt("get projects: %w", err)
	}
	// The order of projects affects worker allocations if projects
	// are entitled to fractional workers. Ensure the project order
	// is stable to keep orchestrator behaviour as stable as possible.
	sort.Strings(projects)

	cfg, err := config.Get(ctx)
	if err != nil {
		status = "failure"
		return errors.Fmt("get service config: %w", err)
	}

	workers := int(cfg.ReclusteringWorkers)
	if workers <= 0 {
		status = "disabled"
		logging.Warningf(ctx, "Reclustering is disabled because configured worker count is zero.")
		return nil
	}

	err = startRuns(ctx, projects, workers)
	if err != nil {
		status = "failure"
		return err
	}
	status = "success"
	return nil
}

// startRuns commences a reclustering run for the given
// projects, splitting the work into a number of shards equal to the
// given number of workers. Each reclustering run takes one minute, and
// the orchestrator should be run just after the start of each minute.
//
// For each project:
//   - One ReclusteringRun record is created defining the minimum versions of
//     algorithms, project configuration and rules the reclustering run
//     aims to achieve.
//   - One ReclusteringShard record is created for every worker allocated
//     to the project, providing a means for the worker to report its progress
//     reclustering a fraction of the project's chunks.
//   - One reclustering task is created for every worker allocated to
//     the project. (Thus starting the worker, which will work on one shard.)
//   - Progress is read from the previous run's ReclusteringShards and
//     used to update the previous run's ReclusteringRun record with
//     the actual progress achieved (towards the reclustering objectives)
//     in that run.
func startRuns(ctx context.Context, projects []string, workers int) error {
	runStart := clock.Now(ctx).Truncate(time.Minute)
	runEnd := runStart.Add(time.Minute)
	err := condenseShardProgressIntoRunProgress(ctx, projects, runEnd)
	if err != nil {
		return errors.Fmt("condensing shard progress into run progress: %w", err)
	}

	workerAllocs, err := projectWorkerAllocations(ctx, projects, workers)
	if err != nil {
		return err
	}

	var errs []error
	for _, project := range projects {
		projectWorkers := workerAllocs[project]
		err := startProjectRun(ctx, project, runEnd, projectWorkers)
		if err != nil {
			// If an error occurs with one project, capture it, but continue
			// to avoid impacting other projects.
			errs = append(errs, errors.Fmt("project %s: %w", project, err))
		}
	}
	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

// workerAllocation represents a set of reclustering workers allocated
// to a project. Each worker represents a single reclustering task that is
// scheduled (and its continuation tasks).
type workerAllocation struct {
	// start is the starting sequence number of the range of workers allocated.
	// The range of workers represented by the allocation
	// is [start, start+count).
	start int
	// count is the number of workers allocated to the project.
	count int
}

// projectWorkerAllocations distributes workers between LUCI projects.
// The workers are allocated proportionately to the number of chunks in each
// project, with a minimum of one worker per project.
func projectWorkerAllocations(ctx context.Context, projects []string, workers int) (map[string]workerAllocation, error) {
	chunksByProject := make(map[string]int64)
	var totalChunks int64

	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	for _, project := range projects {
		estimate, err := state.EstimateChunks(txn, project)
		if err != nil {
			return nil, errors.Fmt("estimating rows for project %s: %w", project, err)
		}
		chunksByProject[project] = int64(estimate)
		totalChunks += int64(estimate)

		chunkGauge.Set(ctx, int64(estimate), project)
	}

	// Each project gets at least one worker. The rest can be divided up
	// according to project size.
	freeWorkers := workers - len(projects)
	if freeWorkers < 0 {
		return nil, errors.New("more projects configured than workers")
	}

	sequence := 1
	result := make(map[string]workerAllocation)
	for _, project := range projects {

		projectChunks := chunksByProject[project]
		// Equiv. to math.Round((projectChunks / totalChunks) * freeWorkers)
		// without floating-point precision issues.
		additionalWorkers := int((projectChunks*int64(freeWorkers) + (totalChunks / 2)) / totalChunks)

		totalChunks -= projectChunks
		freeWorkers -= additionalWorkers

		// Every project gets at least one worker, plus
		// a number of workers depending on it size.
		alloc := workerAllocation{
			start: sequence,
			count: 1 + additionalWorkers,
		}
		sequence += alloc.count
		result[project] = alloc
	}
	return result, nil
}

// startProjectRun starts a new reclustering run for the given project,
// with the specified end time, and number of workers.
func startProjectRun(ctx context.Context, project string, runEnd time.Time, workers workerAllocation) error {
	var progress *metrics
	var newRun *runs.ReclusteringRun
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		lastComplete, err := runs.ReadLastCompleteUpTo(ctx, project, runs.MaxAttemptTimestamp)
		if err != nil {
			return errors.Fmt("read last complete run: %w", err)
		}
		lastRun, err := runs.ReadLastUpTo(ctx, project, runs.MaxAttemptTimestamp)
		if err != nil {
			return errors.Fmt("read last run: %w", err)
		}
		progress = &metrics{
			progress:      float64(int(lastRun.Progress/lastRun.ShardCount)) / 1000.0,
			lastCompleted: lastComplete.AttemptTimestamp,
		}

		// Create the run for the project.
		newRun, err = nextProjectRun(ctx, project, runEnd, workers, lastRun)
		if err != nil {
			return errors.Fmt("create run: %w", err)
		}
		err = runs.Create(ctx, newRun)
		if err != nil {
			return errors.Fmt("create new run: %w", err)
		}

		// Create the shard entries for that run.
		newShards := nextShards(project, runEnd, workers)
		for _, shard := range newShards {
			err := shards.Create(ctx, shard)
			if err != nil {
				return errors.Fmt("create shard: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Export metrics.
	progressGauge.Set(ctx, float64(progress.progress), project)
	workersGauge.Set(ctx, int64(workers.count), project)
	if progress.lastCompleted != runs.StartingEpoch {
		// Only report time since last completion once there is a last completion.
		lastCompletedGauge.Set(ctx, progress.lastCompleted.Unix(), project)
	}

	// Kick off the worker tasks for each of the shards.
	tasks := nextWorkerTasks(newRun, workers)

	// Each task takes around 20 milliseconds (November 2022) to send to
	// cloud tasks. For a project with 500 worker tasks, this could take 10
	// seconds if we performed it serially, so let's parallelise to cut down
	// the time a bit.
	err = parallel.WorkPool(10, func(c chan<- func() error) {
		for _, task := range tasks {
			// Assign to variable to capture current value of loop variable.
			t := task
			c <- func() error {
				err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
					return reclustering.Schedule(ctx, t)
				}, nil)
				return err
			}
		}
	})
	if err != nil {
		return errors.Fmt("schedule workers: %w", err)
	}
	return nil
}

type metrics struct {
	// lastCompleted is the time since the last completed re-clustering.
	// If no reclustering has been completed this is runs.StartingEpoch.
	lastCompleted time.Time
	// progress, measured from 0.0 to 1.0.
	progress float64
}

// condenseShardProgressIntoRunProgress updates the progress of the last
// reclustering runs (if any) based on the per-shard progress reports,
// and then deletes the per-shard progress reports.
func condenseShardProgressIntoRunProgress(ctx context.Context, projects []string, runEnd time.Time) error {
	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	lastAttempt := runEnd.Add(-1 * time.Minute)

	progresses, err := shards.ReadAllProgresses(txn, lastAttempt)
	if err != nil {
		return errors.Fmt("read shard progress: %w", err)
	}
	// Perform in a separate transaction to the read
	// above to avoid this transaction aborting and retrying if a
	// shard has overrun beyond the attempt finish time has updated
	// its progress. Shards will never reverse in progress within a given
	// attempt, so the progress we set on the run will always be a lower bound
	// on the actual value.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, p := range progresses {
			// Note that if there is progress for a run, there must
			// be a run as well, as runs are created with the shards.
			err := runs.UpdateProgress(ctx, p.Project, p.AttemptTimestamp, p.ShardsReported, p.Progress)
			if err != nil {
				return errors.Fmt("updating progress for project %s: %w", p.Project, err)
			}
		}
		// Delete the shard rows, so that we don't clog up the
		// table with millions of useless rows.
		shards.DeleteAll(ctx)
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// nextProjectRun defines the next reclustering run for a project, based on the
// results of the last run and the current configuration, algorithms and rules
// versions.
func nextProjectRun(ctx context.Context, project string, runEnd time.Time, workers workerAllocation, lastRun *runs.ReclusteringRun) (*runs.ReclusteringRun, error) {
	projectCfg, err := config.Project(ctx, project)
	if err != nil {
		return nil, errors.Fmt("get project config: %w", err)
	}
	latestConfigVersion := projectCfg.LastUpdated.AsTime()

	// run.Progress is a value between 0 and 1000 * lastRun.ShardCount.
	progress := int(lastRun.Progress / lastRun.ShardCount)

	newRun := &runs.ReclusteringRun{
		Project:          project,
		AttemptTimestamp: runEnd,
		ShardCount:       int64(workers.count),
		ShardsReported:   0,
		Progress:         0,
	}
	if progress == 1000 {
		rulesVersion, err := rules.ReadVersion(ctx, project)
		if err != nil {
			return nil, err
		}
		// Trigger re-clustering on rule predicate changes,
		// as only the rule predicate matters for determining
		// which clusters a failure is in.
		newRun.RulesVersion = rulesVersion.Predicates
		newRun.AlgorithmsVersion = algorithms.AlgorithmsVersion
		if lastRun.AlgorithmsVersion > newRun.AlgorithmsVersion {
			// Never roll back to an earlier algorithms version. Assume
			// this orchestrator is running old code.
			newRun.AlgorithmsVersion = lastRun.AlgorithmsVersion
		}
		newRun.ConfigVersion = latestConfigVersion
		if lastRun.ConfigVersion.After(newRun.ConfigVersion) {
			// Never roll back to an earlier config version. Assume
			// this orchestrator still has old config cached.
			newRun.ConfigVersion = lastRun.ConfigVersion
		}
	} else {
		// It is foreseeable that re-clustering rules could have changed
		// every time the orchestrator runs. If we update the rules
		// version for each new run, we may be continuously
		// re-clustering chunks early on in the keyspace without ever
		// getting around to later chunks. To ensure progress, and ensure
		// that every chunk gets a fair slice of re-clustering resources,
		// keep the same re-clustering goals until the last run has completed.
		newRun.RulesVersion = lastRun.RulesVersion
		newRun.ConfigVersion = lastRun.ConfigVersion
		newRun.AlgorithmsVersion = lastRun.AlgorithmsVersion
	}
	return newRun, nil
}

// nexShards defines the shard entries needed for each reclustering
// worker to report its progress.
func nextShards(project string, runEnd time.Time, alloc workerAllocation) []shards.ReclusteringShard {
	var result []shards.ReclusteringShard
	for i := 0; i < alloc.count; i++ {
		shard := shards.ReclusteringShard{
			ShardNumber:      int64(alloc.start + i),
			AttemptTimestamp: runEnd,
			Project:          project,
		}
		result = append(result, shard)
	}
	return result
}

// nextWorkerTasks creates reclustering tasks for the given reclustering
// run and worker allocation. Workers are each assigned an equally large slice
// of the keyspace to recluster.
func nextWorkerTasks(newRun *runs.ReclusteringRun, alloc workerAllocation) []*taskspb.ReclusterChunks {
	splits := workerSplits(alloc.count)

	var tasks []*taskspb.ReclusterChunks
	for i := 0; i < alloc.count; i++ {
		splitStart := splits[i]
		splitEnd := splits[i+1]

		attemptStart := newRun.AttemptTimestamp.Add(-1 * time.Minute)

		// Space out the initial progress reporting of each task in the range
		// [0,progressIntervalSeconds). This spreads out some of the progress
		// update load on Spanner.
		reportOffset := time.Duration((int64(worker.ProgressInterval) * int64(i)) / int64(alloc.count))

		task := &taskspb.ReclusterChunks{
			ShardNumber:       int64(alloc.start + i),
			Project:           newRun.Project,
			AttemptTime:       timestamppb.New(newRun.AttemptTimestamp),
			StartChunkId:      splitStart,
			EndChunkId:        splitEnd,
			AlgorithmsVersion: newRun.AlgorithmsVersion,
			RulesVersion:      timestamppb.New(newRun.RulesVersion),
			ConfigVersion:     timestamppb.New(newRun.ConfigVersion),
			State: &taskspb.ReclusterChunkState{
				CurrentChunkId: splitStart,
				NextReportDue:  timestamppb.New(attemptStart.Add(reportOffset)),
			},
		}
		tasks = append(tasks, task)
	}
	return tasks
}

// workerSplits divides the chunk ID key space evenly into the given
// number of partitions. count + 1 entries are returned; with the chunk ID
// range for each partition being the range between two adjacent entries,
// i.e. partition 0 is from result[0] (exclusive) to result[1] (inclusive),
// partition 1 is from result[1] to result[2], and so on.
func workerSplits(count int) []string {
	var result []string
	// "" indicates table start, which is the start of the first partition.
	result = append(result, "")

	var keyspaceSize big.Int
	// keyspaceSize = 1 << 128  (for 128-bits of keyspace).
	keyspaceSize.Lsh(big.NewInt(1), 128)

	for i := range count {
		// Identify the split point between two partitions.
		// split = keyspaceSize * (i + 1) / count
		var split big.Int
		split.Mul(&keyspaceSize, big.NewInt(int64(i+1)))
		split.Div(&split, big.NewInt(int64(count)))

		// Subtract one to adjust for the upper bound being inclusive
		// and not exclusive. (e.g. the last split should be (1 << 128) - 1,
		// which is fffffff .... ffffff in hexadecimal,  not (1 << 128),
		// which is a "1" with 32 zeroes in hexadecimal).
		split.Sub(&split, big.NewInt(1))

		// Convert the split to a hexadecimal string.
		key := split.Text(16)
		if len(key) < 32 {
			// Pad the value with "0"s to get to the 32 hexadecimal
			// character length of a chunk ID.
			key = strings.Repeat("0", 32-len(key)) + key
		}
		result = append(result, key)
	}
	return result
}
