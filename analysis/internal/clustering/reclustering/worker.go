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

	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/clustering/shards"
	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/tracing"
)

const (
	// batchSize is the number of chunks to read from Spanner at a time.
	batchSize = 10

	// The maximum number of concurrent worker goroutines to use to
	// fetch chunks from GCS for reclustering. More parallelism can
	// improve performance, but increases the memory used by each task.
	maxParallelism = 10

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
	ProgressInterval = 5 * time.Second
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
	worker *Worker
	task   *taskspb.ReclusterChunks
	// nextReportDue is the time at which the next progress update is
	// due.
	nextReportDue time.Time
	// currentChunkID is the exclusive lower bound of the range
	// of ChunkIds still to re-cluster.
	currentChunkID string
}

// Do works on a re-clustering task for approximately duration, returning a
// continuation task (if the run end time has not been reached).
//
// Continuation tasks are used to better integrate with GAE autoscaling,
// autoscaling work best when tasks are relatively small (so that work
// can be moved between instances in real time).
func (w *Worker) Do(ctx context.Context, task *taskspb.ReclusterChunks, duration time.Duration) (*taskspb.ReclusterChunks, error) {
	if task.State == nil {
		return nil, errors.New("task does not have state")
	}
	if task.ShardNumber <= 0 {
		return nil, errors.New("task must have valid shard number")
	}
	if task.AlgorithmsVersion <= 0 {
		return nil, errors.New("task must have valid algorithms version")
	}

	runEndTime := task.AttemptTime.AsTime()

	if task.AlgorithmsVersion > algorithms.AlgorithmsVersion {
		return nil, fmt.Errorf("running out-of-date algorithms version (task requires %v, worker running %v)",
			task.AlgorithmsVersion, algorithms.AlgorithmsVersion)
	}

	tctx := &taskContext{
		worker:         w,
		task:           task,
		nextReportDue:  task.State.NextReportDue.AsTime(),
		currentChunkID: task.State.CurrentChunkId,
	}

	// softEndTime is the (soft) deadline for the run.
	softEndTime := clock.Now(ctx).Add(duration)
	if runEndTime.Before(softEndTime) {
		// Stop by the run end time.
		softEndTime = runEndTime
	}

	var done bool
	for clock.Now(ctx).Before(softEndTime) && !done {
		err := retry.Retry(ctx, transient.Only(retry.Default), func() error {
			// Stop harder if retrying after the run end time, to avoid
			// getting stuck in a retry loop if we are running in
			// parallel with another worker.
			if !clock.Now(ctx).Before(runEndTime) {
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
	if softEndTime.Before(runEndTime) && !done {
		continuation = &taskspb.ReclusterChunks{
			ShardNumber:       task.ShardNumber,
			Project:           task.Project,
			AttemptTime:       task.AttemptTime,
			StartChunkId:      task.StartChunkId,
			EndChunkId:        task.EndChunkId,
			AlgorithmsVersion: task.AlgorithmsVersion,
			RulesVersion:      task.RulesVersion,
			ConfigVersion:     task.ConfigVersion,
			State: &taskspb.ReclusterChunkState{
				CurrentChunkId: tctx.currentChunkID,
				NextReportDue:  timestamppb.New(tctx.nextReportDue),
			},
		}
	}
	return continuation, nil
}

// recluster tries to reclusters some chunks, advancing currentChunkID
// as it succeeds. It returns 'true' if all chunks to be re-clustered by
// the reclustering task were completed.
func (t *taskContext) recluster(ctx context.Context) (done bool, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/clustering/reclustering.recluster",
		attribute.String("project", t.task.Project),
		attribute.String("currentChunkID", t.currentChunkID),
	)
	defer func() { tracing.End(s, err) }()

	readOpts := state.ReadNextOptions{
		StartChunkID:      t.currentChunkID,
		EndChunkID:        t.task.EndChunkId,
		AlgorithmsVersion: t.task.AlgorithmsVersion,
		ConfigVersion:     t.task.ConfigVersion.AsTime(),
		RulesVersion:      t.task.RulesVersion.AsTime(),
	}
	entries, err := state.ReadNextN(span.Single(ctx), t.task.Project, readOpts, batchSize)
	if err != nil {
		return false, errors.Annotate(err, "read next chunk state").Err()
	}
	if len(entries) == 0 {
		// We have finished re-clustering.
		err = t.updateProgress(ctx, shards.MaxProgress)
		if err != nil {
			return true, err
		}
		return true, nil
	}

	// Obtain a recent ruleset of at least RulesVersion.
	ruleset, err := Ruleset(ctx, t.task.Project, t.task.RulesVersion.AsTime())
	if err != nil {
		return false, errors.Annotate(err, "obtain ruleset").Err()
	}

	// Obtain a recent configuration of at least ConfigVersion.
	cfg, err := compiledcfg.Project(ctx, t.task.Project, t.task.ConfigVersion.AsTime())
	if err != nil {
		return false, errors.Annotate(err, "obtain config").Err()
	}

	// Prepare updates for each chunk in the batch. Parallelise to
	// allow us to fetch chunks from GCS faster.
	updates := make([]*PendingUpdate, len(entries))
	err = parallel.WorkPool(maxParallelism, func(c chan<- func() error) {
		for i, entry := range entries {
			// Capture value of loop variables.
			c <- func() error {

				// Read the test results from GCS.
				chunk, err := t.worker.chunkStore.Get(ctx, t.task.Project, entry.ObjectID)
				if err != nil {
					return errors.Annotate(err, "read chunk").Err()
				}

				// Re-cluster the test results in spanner, then export
				// the re-clustering to BigQuery for analysis.
				update, err := PrepareUpdate(ctx, ruleset, cfg, chunk, entry)
				if err != nil {
					return errors.Annotate(err, "re-cluster chunk").Err()
				}

				updates[i] = update
				return nil
			}
		}
	})
	if err != nil {
		return false, err
	}

	pendingUpdates := NewPendingUpdates(ctx)

	for i, update := range updates {
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
			t.currentChunkID = update.Chunk.ChunkID

			if err := t.calculateAndReportProgress(ctx); err != nil {
				return false, err
			}
		}
	}

	// More to do.
	return false, nil
}

// calculateAndReportProgress reports progress on the shard, based on the current
// value of t.currentChunkID. It can only be used to report interim progress (it
// will never report a progress value of 1000).
func (t *taskContext) calculateAndReportProgress(ctx context.Context) (err error) {
	// Manage contention on the ReclusteringRun row by only periodically
	// reporting progress.
	if clock.Now(ctx).After(t.nextReportDue) {
		progress, err := calculateProgress(t.task, t.currentChunkID)
		if err != nil {
			return errors.Annotate(err, "calculate progress").Err()
		}

		err = t.updateProgress(ctx, progress)
		if err != nil {
			return err
		}
		t.nextReportDue = t.nextReportDue.Add(ProgressInterval)
	}
	return nil
}

// updateProgress sets progress on the shard.
func (t *taskContext) updateProgress(ctx context.Context, value int) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/clustering/reclustering.updateProgress")
	defer func() { tracing.End(s, err) }()

	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		err = shards.UpdateProgress(ctx, t.task.ShardNumber, t.task.AttemptTime.AsTime(), value)
		if err != nil {
			return errors.Annotate(err, "update progress").Err()
		}
		return nil
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// If the row for the shard has been deleted (i.e. because
			// we have overrun the end of our reclustering run), drop
			// the progress update.
			return nil
		}
		return err
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

	// progress = (((nextID - 1) - startID) * shards.MaxProgress) / (endID - startID)
	var numerator big.Int
	numerator.Sub(nextID, big.NewInt(1))
	numerator.Sub(&numerator, startID)
	numerator.Mul(&numerator, big.NewInt(shards.MaxProgress))

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
