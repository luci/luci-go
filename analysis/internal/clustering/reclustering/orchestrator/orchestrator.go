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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/span"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	worker "go.chromium.org/luci/analysis/internal/clustering/reclustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
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

	projectCfg, err := config.Projects(ctx)
	if err != nil {
		status = "failure"
		return errors.Annotate(err, "get projects config").Err()
	}
	var projects []string
	for project := range projectCfg {
		projects = append(projects, project)
	}
	// The order of projects affects worker allocations if projects
	// are entitled to fractional workers. Ensure the project order
	// is stable to keep orchestrator behaviour as stable as possible.
	sort.Strings(projects)

	cfg, err := config.Get(ctx)
	if err != nil {
		status = "failure"
		return errors.Annotate(err, "get service config").Err()
	}

	workers := int(cfg.ReclusteringWorkers)
	intervalMinutes := int(cfg.ReclusteringIntervalMinutes)
	if workers <= 0 {
		status = "disabled"
		logging.Warningf(ctx, "Reclustering is disabled because configured worker count is zero.")
		return nil
	}
	if intervalMinutes <= 0 {
		status = "disabled"
		logging.Warningf(ctx, "Reclustering is disabled because configured reclustering interval is zero.")
		return nil
	}

	err = orchestrateWithOptions(ctx, projects, workers, intervalMinutes)
	if err != nil {
		status = "failure"
		return err
	}
	status = "success"
	return nil
}

func orchestrateWithOptions(ctx context.Context, projects []string, workers, intervalMins int) error {
	currentMinute := clock.Now(ctx).Truncate(time.Minute)
	intervalDuration := time.Duration(intervalMins) * time.Minute
	attemptStart := clock.Now(ctx).Truncate(intervalDuration)
	if attemptStart != currentMinute {
		logging.Infof(ctx, "Orchestrator ran, but determined the current run start %v"+
			" does not match the current minute %v.", attemptStart, currentMinute)
		return nil
	}
	attemptEnd := attemptStart.Add(intervalDuration)

	workerCounts, err := projectWorkerCounts(ctx, projects, workers)
	if err != nil {
		return err
	}

	var errs []error
	for _, project := range projects {
		projectWorkers := workerCounts[project]
		err := orchestrateProject(ctx, project, attemptStart, attemptEnd, projectWorkers)
		if err != nil {
			// If an error occurs with one project, capture it, but continue
			// to avoid impacting other projects.
			errs = append(errs, errors.Annotate(err, "project %s", project).Err())
		}
	}
	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

// projectWorkerCounts distributes workers between LUCI projects.
// The workers are allocated proportionately to the number of chunks in each
// project, with a minimum of one worker per project.
func projectWorkerCounts(ctx context.Context, projects []string, workers int) (map[string]int, error) {
	chunksByProject := make(map[string]int64)
	var totalChunks int64

	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	for _, project := range projects {
		estimate, err := state.EstimateChunks(txn, project)
		if err != nil {
			return nil, errors.Annotate(err, "estimating rows for project %s", project).Err()
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

	result := make(map[string]int)
	for _, project := range projects {

		projectChunks := chunksByProject[project]
		// Equiv. to math.Round((projectChunks / totalChunks) * freeWorkers)
		// without floating-point precision issues.
		additionalWorkers := int((projectChunks*int64(freeWorkers) + (totalChunks / 2)) / totalChunks)

		totalChunks -= projectChunks
		freeWorkers -= additionalWorkers

		// Every project gets at least one worker, plus
		// a number of workers depending on it size.
		result[project] = 1 + additionalWorkers
	}
	return result, nil
}

// orchestrateProject starts a new reclustering run for the given project,
// with the specified start and end time, and number of workers.
func orchestrateProject(ctx context.Context, project string, attemptStart, attemptEnd time.Time, workers int) error {
	projectCfg, err := config.Project(ctx, project)
	if err != nil {
		return errors.Annotate(err, "get project config").Err()
	}
	configVersion := projectCfg.LastUpdated.AsTime()

	var metrics *metrics
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		var err error
		metrics, err = createProjectRun(ctx, project, attemptStart, attemptEnd, configVersion, workers)
		return err
	})
	if err != nil {
		return errors.Annotate(err, "create run").Err()
	}

	// Export metrics.
	progressGauge.Set(ctx, float64(metrics.progress), project)
	workersGauge.Set(ctx, int64(workers), project)
	if metrics.lastCompleted != runs.StartingEpoch {
		// Only report time since last completion once there is a last completion.
		lastCompletedGauge.Set(ctx, metrics.lastCompleted.Unix(), project)
	}

	err = scheduleWorkers(ctx, project, attemptStart, attemptEnd, workers)
	if err != nil {
		return errors.Annotate(err, "schedule workers").Err()
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

// createProjectRun creates a new run entry for a project, returning whether
// the previous run achieved its re-clustering goal (and any errors).
func createProjectRun(ctx context.Context, project string, attemptStart, attemptEnd, latestConfigVersion time.Time, workers int) (*metrics, error) {
	lastComplete, err := runs.ReadLastComplete(ctx, project)
	if err != nil {
		return nil, errors.Annotate(err, "read last complete run").Err()
	}

	lastRun, err := runs.ReadLast(ctx, project)
	if err != nil {
		return nil, errors.Annotate(err, "read last run").Err()
	}

	// run.Progress is a value between 0 and 1000 * lastRun.ShardCount.
	progress := int(lastRun.Progress / lastRun.ShardCount)

	if lastRun.AttemptTimestamp.After(attemptStart) {
		return nil, errors.New("an attempt which overlaps the proposed attempt already exists")
	}
	newRun := &runs.ReclusteringRun{
		Project:          project,
		AttemptTimestamp: attemptEnd,
		ShardCount:       int64(workers),
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
	err = runs.Create(ctx, newRun)
	if err != nil {
		return nil, errors.Annotate(err, "create new run").Err()
	}
	metrics := &metrics{
		progress: float64(progress) / 1000.0,
	}
	metrics.lastCompleted = lastComplete.AttemptTimestamp
	return metrics, err
}

// scheduleWorkers creates reclustering tasks for the given project
// and attempt. Workers are each assigned an equally large slice
// of the keyspace to recluster.
func scheduleWorkers(ctx context.Context, project string, attemptStart time.Time, attemptEnd time.Time, count int) error {
	splits := workerSplits(count)
	for i := 0; i < count; i++ {
		start := splits[i]
		end := splits[i+1]

		// Space out the initial progress reporting of each task in the range
		// [0,progressIntervalSeconds).
		// This avoids creating contention on the ReclusteringRuns row.
		reportOffset := time.Duration((int64(worker.ProgressInterval) / int64(count)) * int64(i))

		task := &taskspb.ReclusterChunks{
			Project:      project,
			AttemptTime:  timestamppb.New(attemptEnd),
			StartChunkId: start,
			EndChunkId:   end,
			State: &taskspb.ReclusterChunkState{
				CurrentChunkId:       start,
				NextReportDue:        timestamppb.New(attemptStart.Add(reportOffset)),
				ReportedOnce:         false,
				LastReportedProgress: 0,
			},
		}
		err := reclustering.Schedule(ctx, task)
		if err != nil {
			return err
		}
	}
	return nil
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

	for i := 0; i < count; i++ {
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
