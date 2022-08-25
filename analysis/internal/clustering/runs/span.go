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

package runs

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	spanutil "go.chromium.org/luci/analysis/internal/span"
)

// ReclusteringRun contains the details of a runs used to re-cluster
// test results.
type ReclusteringRun struct {
	// The LUCI Project for which this rule is defined.
	Project string
	// The attempt. This is the time the orchestrator run ends.
	AttemptTimestamp time.Time
	// The minimum algorithms version this reclustering run is trying
	// to achieve. Chunks with an AlgorithmsVersion less than this
	// value are eligible to be re-clustered.
	AlgorithmsVersion int64
	// The minimum config version the reclustering run is trying to achieve.
	// Chunks with a ConfigVersion less than this value are eligible to be
	// re-clustered.
	ConfigVersion time.Time
	// The minimum rules version the reclustering run is trying to achieve.
	// Chunks with a RulesVersion less than this value are eligible to be
	// re-clustered.
	RulesVersion time.Time
	// The number of shards created for this run (for this LUCI project).
	ShardCount int64
	// The number of shards that have reported progress (at least once).
	// When this is equal to ShardCount, readers can have confidence Progress
	// is a reasonable reflection of the progress made reclustering
	// this project. Until then, it is a loose lower-bound.
	ShardsReported int64
	// The progress. This is a value between 0 and 1000*ShardCount.
	Progress int64
}

// NotFound is the error returned by Read if the row could not be found.
var NotFound = errors.New("reclustering run row not found")

// StartingEpoch is the earliest valid run attempt time.
var StartingEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// Read reads the run with the given attempt timestamp in the given LUCI
// project. If the row does not exist, the error NotFound is returned.
func Read(ctx context.Context, projectID string, attemptTimestamp time.Time) (*ReclusteringRun, error) {
	whereClause := `AttemptTimestamp = @attemptTimestamp`
	params := map[string]interface{}{
		"attemptTimestamp": attemptTimestamp,
	}
	r, err := readLastWhere(ctx, projectID, whereClause, params)
	if err != nil {
		return nil, errors.Annotate(err, "query run").Err()
	}
	if r == nil {
		return nil, NotFound
	}
	return r, nil
}

// ReadLast reads the last run in the given LUCI project. If no row exists,
// a fake run is returned with the following details:
// - Project matching the requested Project ID.
// - AttemptTimestamp of StartingEpoch.
// - AlgorithmsVersion of 1.
// - ConfigVersion of clusteringcfg.StartingEpoch.
// - RulesVersion of rules.StartingEpoch.
// - ShardCount and ShardsReported of 1.
// - Progress of 1000.
func ReadLast(ctx context.Context, projectID string) (*ReclusteringRun, error) {
	whereClause := `TRUE`
	r, err := readLastWhere(ctx, projectID, whereClause, nil)
	if err != nil {
		return nil, errors.Annotate(err, "query last run").Err()
	}
	if r == nil {
		r = fakeLastRow(projectID)
	}
	return r, nil
}

// ReadLastWithProgress reads the last run with progress in the given LUCI
// project. If no row exists, a fake row is returned; see ReadLast for details.
func ReadLastWithProgress(ctx context.Context, projectID string) (*ReclusteringRun, error) {
	whereClause := `ShardsReported = ShardCount`
	r, err := readLastWhere(ctx, projectID, whereClause, nil)
	if err != nil {
		return nil, errors.Annotate(err, "query last run").Err()
	}
	if r == nil {
		r = fakeLastRow(projectID)
	}
	return r, nil
}

// ReadLastComplete reads the last run that completed in the given LUCI
// project. If no row exists, a fake row is returned; see ReadLast for details.
func ReadLastComplete(ctx context.Context, projectID string) (*ReclusteringRun, error) {
	whereClause := `Progress = (ShardCount * 1000)`
	r, err := readLastWhere(ctx, projectID, whereClause, nil)
	if err != nil {
		return nil, errors.Annotate(err, "query last run").Err()
	}
	if r == nil {
		r = fakeLastRow(projectID)
	}
	return r, nil
}

func fakeLastRow(projectID string) *ReclusteringRun {
	return &ReclusteringRun{
		Project:           projectID,
		AttemptTimestamp:  StartingEpoch,
		AlgorithmsVersion: 1,
		ConfigVersion:     config.StartingEpoch,
		RulesVersion:      rules.StartingEpoch,
		ShardCount:        1,
		ShardsReported:    1,
		Progress:          1000,
	}
}

// readLastWhere reads the last run matching the given where clause,
// substituting params for any SQL parameters used in that clause.
func readLastWhere(ctx context.Context, projectID string, whereClause string, params map[string]interface{}) (*ReclusteringRun, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  AttemptTimestamp, ConfigVersion, RulesVersion,
		  AlgorithmsVersion, ShardCount, ShardsReported, Progress
		FROM ReclusteringRuns
		WHERE Project = @projectID AND (` + whereClause + `)
		ORDER BY AttemptTimestamp DESC
		LIMIT 1
	`)
	for k, v := range params {
		stmt.Params[k] = v
	}
	stmt.Params["projectID"] = projectID

	it := span.Query(ctx, stmt)
	rs := []*ReclusteringRun{}
	err := it.Do(func(r *spanner.Row) error {
		var attemptTimestamp, rulesVersion, configVersion time.Time
		var algorithmsVersion, shardCount, shardsReported, progress int64
		err := r.Columns(
			&attemptTimestamp, &configVersion, &rulesVersion,
			&algorithmsVersion, &shardCount, &shardsReported, &progress,
		)
		if err != nil {
			return errors.Annotate(err, "read run row").Err()
		}

		run := &ReclusteringRun{
			Project:           projectID,
			AttemptTimestamp:  attemptTimestamp,
			AlgorithmsVersion: algorithmsVersion,
			ConfigVersion:     configVersion,
			RulesVersion:      rulesVersion,
			ShardCount:        shardCount,
			ShardsReported:    shardsReported,
			Progress:          progress,
		}
		rs = append(rs, run)
		return nil
	})
	if len(rs) > 0 {
		return rs[0], err
	}
	return nil, err
}

// Create inserts a new reclustering run.
func Create(ctx context.Context, r *ReclusteringRun) error {
	if err := validateRun(r); err != nil {
		return err
	}
	ms := spanutil.InsertMap("ReclusteringRuns", map[string]interface{}{
		"Project":           r.Project,
		"AttemptTimestamp":  r.AttemptTimestamp,
		"AlgorithmsVersion": r.AlgorithmsVersion,
		"ConfigVersion":     r.ConfigVersion,
		"RulesVersion":      r.RulesVersion,
		"ShardCount":        r.ShardCount,
		"ShardsReported":    r.ShardsReported,
		"Progress":          r.Progress,
	})
	span.BufferWrite(ctx, ms)
	return nil
}

func validateRun(r *ReclusteringRun) error {
	switch {
	case !config.ProjectRe.MatchString(r.Project):
		return errors.New("project must be valid")
	case r.AttemptTimestamp.Before(StartingEpoch):
		return errors.New("attempt timestamp must be valid")
	case r.AlgorithmsVersion <= 0:
		return errors.New("algorithms version must be valid")
	case r.ConfigVersion.Before(config.StartingEpoch):
		return errors.New("config version must be valid")
	case r.RulesVersion.Before(rules.StartingEpoch):
		return errors.New("rules version must be valid")
	case r.ShardCount <= 0:
		return errors.New("shard count must be valid")
	case r.ShardsReported < 0 || r.ShardsReported > r.ShardCount:
		return errors.New("shards reported must be valid")
	case r.Progress < 0 || r.Progress > (r.ShardCount*1000):
		return errors.New("progress must be valid")
	}
	return nil
}

// reportProgress adds progress to a particular run. To ensure correct
// usage, this should only be called from ProgressToken.
func reportProgress(ctx context.Context, projectID string, attemptTimestamp time.Time, firstReport bool, deltaProgress int) error {
	stmt := spanner.NewStatement(`
	  UPDATE ReclusteringRuns
	  SET ShardsReported = ShardsReported + @deltaShardsReported,
	      Progress = Progress + @deltaProgress
	  WHERE Project = @projectID AND AttemptTimestamp = @attemptTimestamp
	`)
	deltaShardsReported := 0
	if firstReport {
		deltaShardsReported = 1
	}
	stmt.Params["deltaShardsReported"] = deltaShardsReported
	stmt.Params["deltaProgress"] = deltaProgress
	stmt.Params["projectID"] = projectID
	stmt.Params["attemptTimestamp"] = attemptTimestamp
	_, err := span.Update(ctx, stmt)
	return err
}
