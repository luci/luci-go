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

package reclustering

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/trace"
	"go.chromium.org/luci/server/span"

	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	// batchSize is the number of chunks to read from Spanner at a time.
	batchSize = 10

	// TargetTaskDuration is the desired duration of a re-clustering task.
	// If a task completes before the reclustering run has completed, a
	// continuation task will be scheduled.
	//
	// Longer durations will incur lower task queuing/re-queueing overhead,
	// but limit the ability of autoscaling to move tasks between instances
	// in response to load.
	TargetTaskDuration = 2 * time.Second

	// ProgressInterval is the amount of time between progress updates.
	//
	// Note that this is the frequency at which updates should
	// be reported for a shard of work; individual tasks are usually
	// much shorter lived and consequently most will not report any progress
	// (unless it is time for the shard to report progress again).
	ProgressInterval = 30 * time.Second
)

// ChunkStore is the interface for the blob store archiving chunks of test
// results for later re-clustering.
type ChunkStore interface {
	// Get retrieves the chunk with the specified object ID and returns it.
	Get(ctx context.Context, project, objectID string) (*cpb.Chunk, error)
}

// Worker provides methods to process re-clustering tasks. It is safe to be
// used by multiple threads concurrently.
type Worker struct {
	chunkStore ChunkStore
	analysis   Analysis
}

// NewWorker initialises a new Worker.
func NewWorker(chunkStore ChunkStore, analysis Analysis) *Worker {
	return &Worker{
		chunkStore: chunkStore,
		analysis:   analysis,
	}
}

// taskContext provides objects relevant to working on a particular
// re-clustering task.
type taskContext struct {
	worker        *Worker
	task          *taskspb.ReclusterChunks
	run           *runs.ReclusteringRun
	progressToken *runs.ProgressToken
	// nextReportDue is the time at which the next progress update is
	// due.
	nextReportDue time.Time
	// currentChunkID is the exclusive lower bound of the range
	// of ChunkIds still to re-cluster.
	currentChunkID string
}

// Do works on a re-clustering task for approximately duration, returning a
// continuation task (if the attempt end time has not been reached).
//
// Continuation tasks are used to better integrate with GAE autoscaling,
// autoscaling work best when tasks are relatively small (so that work
// can be moved between instances in real time).
func (w *Worker) Do(ctx context.Context, task *taskspb.ReclusterChunks, duration time.Duration) (*taskspb.ReclusterChunks, error) {
	if task.State == nil {
		return nil, errors.New("task does not have state")
	}

	attemptTime := task.AttemptTime.AsTime()

	run, err := runs.Read(span.Single(ctx), task.Project, attemptTime)
	if err != nil {
		return nil, errors.Annotate(err, "read run for task").Err()
	}

	if run.AlgorithmsVersion > algorithms.AlgorithmsVersion {
		return nil, fmt.Errorf("running out-of-date algorithms version (task requires %v, worker running %v)",
			run.AlgorithmsVersion, algorithms.AlgorithmsVersion)
	}

	progressState := &runs.ProgressState{
		ReportedOnce:         task.State.ReportedOnce,
		LastReportedProgress: int(task.State.LastReportedProgress),
	}

	pt := runs.NewProgressToken(task.Project, attemptTime, progressState)
	tctx := &taskContext{
		worker:         w,
		task:           task,
		run:            run,
		progressToken:  pt,
		nextReportDue:  task.State.NextReportDue.AsTime(),
		currentChunkID: task.State.CurrentChunkId,
	}

	// finishTime is the (soft) deadline for the run.
	finishTime := clock.Now(ctx).Add(duration)
	if attemptTime.Before(finishTime) {
		// Stop by the attempt time.
		finishTime = attemptTime
	}

	var done bool
	for clock.Now(ctx).Before(finishTime) && !done {
		err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
			// Stop harder if retrying after the attemptTime, to avoid
			// getting stuck in a retry loop if we are running in
			// parallel with another worker.
			if !clock.Now(ctx).Before(attemptTime) {
				return nil
			}
			var err error
			done, err = tctx.recluster(ctx)
			return err
		}, nil)
		if err != nil {
			return nil, err
		}
	}

	var continuation *taskspb.ReclusterChunks
	if finishTime.Before(attemptTime) && !done {
		ps, err := pt.ExportState()
		if err != nil {
			return nil, err
		}
		continuation = &taskspb.ReclusterChunks{
			Project:      task.Project,
			AttemptTime:  task.AttemptTime,
			StartChunkId: task.StartChunkId,
			EndChunkId:   task.EndChunkId,
			State: &taskspb.ReclusterChunkState{
				CurrentChunkId:       tctx.currentChunkID,
				NextReportDue:        timestamppb.New(tctx.nextReportDue),
				ReportedOnce:         ps.ReportedOnce,
				LastReportedProgress: int64(ps.LastReportedProgress),
			},
		}
	}
	return continuation, nil
}

// recluster tries to reclusters some chunks, advancing currentChunkID
// as it succeeds. It returns 'true' if all chunks to be re-clustered by
// the reclustering task were completed.
func (t *taskContext) recluster(ctx context.Context) (done bool, err error) {
	ctx, s := trace.StartSpan(ctx, "go.chromium.org/luci/analysis/internal/clustering/reclustering.recluster")
	s.Attribute("project", t.task.Project)
	s.Attribute("currentChunkID", t.currentChunkID)
	defer func() { s.End(err) }()

	readOpts := state.ReadNextOptions{
		StartChunkID:      t.currentChunkID,
		EndChunkID:        t.task.EndChunkId,
		AlgorithmsVersion: t.run.AlgorithmsVersion,
		ConfigVersion:     t.run.ConfigVersion,
		RulesVersion:      t.run.RulesVersion,
	}
	entries, err := state.ReadNextN(span.Single(ctx), t.task.Project, readOpts, batchSize)
	if err != nil {
		return false, errors.Annotate(err, "read next chunk state").Err()
	}
	if len(entries) == 0 {
		// We have finished re-clustering.
		err = t.progressToken.ReportProgress(ctx, 1000)
		if err != nil {
			return true, errors.Annotate(err, "report progress").Err()
		}
		return true, nil
	}

	pendingUpdates := NewPendingUpdates(ctx)

	for i, entry := range entries {
		// Read the test results from GCS.
		chunk, err := t.worker.chunkStore.Get(ctx, t.task.Project, entry.ObjectID)
		if err != nil {
			return false, errors.Annotate(err, "read chunk").Err()
		}

		// Obtain a recent ruleset of at least RulesVersion.
		ruleset, err := Ruleset(ctx, t.task.Project, t.run.RulesVersion)
		if err != nil {
			return false, errors.Annotate(err, "obtain ruleset").Err()
		}

		// Obtain a recent configuration of at least ConfigVersion.
		cfg, err := compiledcfg.Project(ctx, t.task.Project, t.run.ConfigVersion)
		if err != nil {
			return false, errors.Annotate(err, "obtain config").Err()
		}

		// Re-cluster the test results in spanner, then export
		// the re-clustering to BigQuery for analysis.
		update, err := PrepareUpdate(ctx, ruleset, cfg, chunk, entry)
		if err != nil {
			return false, errors.Annotate(err, "re-cluster chunk").Err()
		}

		pendingUpdates.Add(update)

		if pendingUpdates.ShouldApply(ctx) || (i == len(entries)-1) {
			if err := pendingUpdates.Apply(ctx, t.worker.analysis); err != nil {
				if err == UpdateRaceErr {
					// Our update raced with another update.
					// This is retriable if we re-read the chunk again.
					err = transient.Tag.Apply(err)
				}
				return false, err
			}
			pendingUpdates = NewPendingUpdates(ctx)

			// Advance our position only on successful commit.
			t.currentChunkID = entry.ChunkID

			if err := t.reportProgress(ctx); err != nil {
				return false, err
			}
		}
	}

	// More to do.
	return false, nil
}

// reportProgress reports progress on the task, based on the current value
// of t.currentChunkID. It can only be used to report interim progress (it
// will never report a progress value of 1000).
func (t *taskContext) reportProgress(ctx context.Context) error {
	// Manage contention on the ReclusteringRun row by only periodically
	// reporting progress.
	if clock.Now(ctx).After(t.nextReportDue) {
		progress, err := calculateProgress(t.task, t.currentChunkID)
		if err != nil {
			return errors.Annotate(err, "calculate progress").Err()
		}

		err = t.progressToken.ReportProgress(ctx, progress)
		if err != nil {
			return errors.Annotate(err, "report progress").Err()
		}
		t.nextReportDue = t.nextReportDue.Add(ProgressInterval)
	}
	return nil
}

// calculateProgress calculates the progress of the worker through the task.
// Progress is the proportion of the keyspace re-clustered, as a value between
// 0 and 1000 (i.e. 0 = 0%, 1000 = 100.0%).
// 1000 is never returned by this method as the value passed is the nextChunkID
// (i.e. the next chunkID to re-cluster), not the last completed chunk ID,
// which implies progress is not complete.
func calculateProgress(task *taskspb.ReclusterChunks, nextChunkID string) (int, error) {
	nextID, err := chunkIDAsBigInt(nextChunkID)
	if err != nil {
		return 0, err
	}
	startID, err := chunkIDAsBigInt(task.StartChunkId)
	if err != nil {
		return 0, err
	}
	endID, err := chunkIDAsBigInt(task.EndChunkId)
	if err != nil {
		return 0, err
	}
	if startID.Cmp(endID) >= 0 {
		return 0, fmt.Errorf("end chunk ID %q is before or equal to start %q", task.EndChunkId, task.StartChunkId)
	}
	if nextID.Cmp(startID) <= 0 {
		// Start is exclusive, not inclusive.
		return 0, fmt.Errorf("next chunk ID %q is before or equal to start %q", nextChunkID, task.StartChunkId)
	}
	if nextID.Cmp(endID) > 0 {
		return 0, fmt.Errorf("next chunk ID %q is after end %q", nextChunkID, task.EndChunkId)
	}

	// progress = (((nextID - 1) - startID) * 1000) / (endID - startID)
	var numerator big.Int
	numerator.Sub(nextID, big.NewInt(1))
	numerator.Sub(&numerator, startID)
	numerator.Mul(&numerator, big.NewInt(1000))

	var denominator big.Int
	denominator.Sub(endID, startID)

	var result big.Int
	result.Div(&numerator, &denominator)

	return int(result.Uint64()), nil
}

// chunkIDAsBigInt represents a 128-bit chunk ID
// (normally represented as 32 lowercase hexadecimal characters)
// as a big.Int.
func chunkIDAsBigInt(chunkID string) (*big.Int, error) {
	if chunkID == "" {
		// "" indicates start of table. This is one before
		// ID 00000 .... 00000.
		return big.NewInt(-1), nil
	}
	idBytes, err := hex.DecodeString(chunkID)
	if err != nil {
		return nil, err
	}
	id := big.NewInt(0)
	id.SetBytes(idBytes)
	return id, nil
}
