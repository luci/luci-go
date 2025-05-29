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

// Package shards provides methods to access the ReclusteringShards
// Spanner table. The table is used by reclustering shards to report progress.
package shards

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/pbutil"
)

// ReclusteringShard is used to for shards to report progress re-clustering
// test results.
type ReclusteringShard struct {
	// A unique number assigned to shard. Shards are numbered sequentially,
	// starting from one.
	ShardNumber int64
	// The attempt. This is the time the orchestrator run ends.
	AttemptTimestamp time.Time
	// The LUCI Project the shard is doing reclustering for.
	Project string
	// The progress. This is a value between 0 and 1000. If this is NULL,
	// it means progress has not yet been reported by the shard.
	Progress spanner.NullInt64
}

// ReclusteringProgress is the result of reading the progress of a project's
// shards for one reclustering attempt.
type ReclusteringProgress struct {
	// The LUCI Project.
	Project string
	// The attempt. This is the time the orchestrator run ends.
	AttemptTimestamp time.Time
	// The number of shards running for the project.
	ShardCount int64
	// The number of shards which have reported progress.
	ShardsReported int64
	// The total progress reported for the project. This is a value
	// between 0 and 1000*ShardCount.
	Progress int64
}

// StartingEpoch is the earliest valid run attempt time.
var StartingEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// MaxProgress is the maximum progress value for a shard, corresponding to
// 100% complete reclustering.
const MaxProgress = 1000

// ReadProgress reads all the progress of reclustering the given
// LUCI Project, for the given attempt timestamp.
func ReadProgress(ctx context.Context, project string, attemptTimestamp time.Time) (ReclusteringProgress, error) {
	whereClause := "Project = @project AND AttemptTimestamp = @attemptTimestamp"
	params := map[string]any{
		"project":          project,
		"attemptTimestamp": attemptTimestamp,
	}
	progress, err := readProgressWhere(ctx, whereClause, params)
	if err != nil {
		return ReclusteringProgress{}, err
	}
	if len(progress) == 0 {
		// No progress available.
		return ReclusteringProgress{
			Project:          project,
			AttemptTimestamp: attemptTimestamp,
		}, nil
	}
	return progress[0], nil
}

// ReadAllProgresses reads reclustering progress for ALL
// projects with shards, for the given attempt timestamp.
func ReadAllProgresses(ctx context.Context, attemptTimestamp time.Time) ([]ReclusteringProgress, error) {
	whereClause := "AttemptTimestamp = @attemptTimestamp"
	params := map[string]any{
		"attemptTimestamp": attemptTimestamp,
	}
	return readProgressWhere(ctx, whereClause, params)
}

// readProgressWhere reads reclustering progress satisfying the given
// where clause.
func readProgressWhere(ctx context.Context, whereClause string, params map[string]any) ([]ReclusteringProgress, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  AttemptTimestamp,
		  Project,
		  SUM(Progress) as Progress,
		  COUNT(1) as ShardCount,
		  COUNTIF(Progress IS NOT NULL) as ShardsReported,
		FROM ReclusteringShards
		WHERE (` + whereClause + `)
		GROUP BY AttemptTimestamp, Project
		ORDER BY AttemptTimestamp, Project
	`)
	for k, v := range params {
		stmt.Params[k] = v
	}

	it := span.Query(ctx, stmt)
	results := []ReclusteringProgress{}
	err := it.Do(func(r *spanner.Row) error {
		var attemptTimestamp time.Time
		var project string
		var progress spanner.NullInt64
		var shardCount, shardsReported int64
		err := r.Columns(
			&attemptTimestamp, &project, &progress, &shardCount, &shardsReported,
		)
		if err != nil {
			return errors.Fmt("read shard row: %w", err)
		}

		result := ReclusteringProgress{
			AttemptTimestamp: attemptTimestamp,
			Project:          project,
			Progress:         progress.Int64,
			ShardCount:       shardCount,
			ShardsReported:   shardsReported,
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ReadAll reads all reclustering shards.
// For testing use only.
func ReadAll(ctx context.Context) ([]ReclusteringShard, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  ShardNumber, AttemptTimestamp, Project, Progress
		FROM ReclusteringShards
		ORDER BY ShardNumber
	`)

	it := span.Query(ctx, stmt)
	results := []ReclusteringShard{}
	err := it.Do(func(r *spanner.Row) error {
		var shardNumber int64
		var attemptTimestamp time.Time
		var project string
		var progress spanner.NullInt64
		err := r.Columns(
			&shardNumber, &attemptTimestamp, &project, &progress,
		)
		if err != nil {
			return errors.Fmt("read shard row: %w", err)
		}

		shard := ReclusteringShard{
			ShardNumber:      shardNumber,
			AttemptTimestamp: attemptTimestamp,
			Project:          project,
			Progress:         progress,
		}
		results = append(results, shard)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// Create inserts a new reclustering shard without progress.
func Create(ctx context.Context, r ReclusteringShard) error {
	if err := validateShard(r); err != nil {
		return err
	}
	ms := spanutil.InsertMap("ReclusteringShards", map[string]any{
		"ShardNumber":      r.ShardNumber,
		"Project":          r.Project,
		"AttemptTimestamp": r.AttemptTimestamp,
	})
	span.BufferWrite(ctx, ms)
	return nil
}

func validateShard(r ReclusteringShard) error {
	if err := pbutil.ValidateProject(r.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
	switch {
	case r.ShardNumber < 1:
		return errors.New("shard number must be a positive integer")
	case r.AttemptTimestamp.Before(StartingEpoch):
		return errors.New("attempt timestamp must be valid")
	}
	return nil
}

// UpdateProgress updates the progress on a particular shard.
// Clients should be mindful of the fact that if they are late
// to update the progress, the entry for a shard may no longer exist.
func UpdateProgress(ctx context.Context, shardNumber int64, attemptTimestamp time.Time, progress int) error {
	if progress < 0 || progress > MaxProgress {
		return errors.Fmt("progress, if set, must be a value between 0 and %v", MaxProgress)
	}
	ms := spanutil.UpdateMap("ReclusteringShards", map[string]any{
		"ShardNumber":      shardNumber,
		"AttemptTimestamp": attemptTimestamp,
		"Progress":         progress,
	})
	span.BufferWrite(ctx, ms)
	return nil
}

// DeleteAll deletes all reclustering shards.
func DeleteAll(ctx context.Context) {
	m := spanner.Delete("ReclusteringShards", spanner.AllKeys())
	span.BufferWrite(ctx, m)
}
