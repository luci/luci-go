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
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
)

const testProject = "myproject"

// RunBuilder provides methods to build a reclustering run
// for testing.
type RunBuilder struct {
	run         *ReclusteringRun
	progressSet bool
}

// NewRun starts building a new Run, for testing.
func NewRun(uniqifier int) *RunBuilder {
	run := &ReclusteringRun{
		Project:           testProject,
		AttemptTimestamp:  time.Date(2010, time.January, 1, 1, 0, 0, uniqifier, time.UTC),
		AlgorithmsVersion: int64(uniqifier + 1),
		ConfigVersion:     time.Date(2011, time.January, 1, 1, 0, 0, uniqifier, time.UTC),
		RulesVersion:      time.Date(2012, time.January, 1, 1, 0, 0, uniqifier, time.UTC),
		ShardCount:        int64(uniqifier + 1),
		ShardsReported:    int64(uniqifier / 2),
		Progress:          int64(uniqifier) * 500,
	}
	return &RunBuilder{
		run: run,
	}
}

// WithProject specifies the project to use on the run.
func (b *RunBuilder) WithProject(project string) *RunBuilder {
	b.run.Project = project
	return b
}

// WithAttemptTimestamp specifies the attempt timestamp to use on the run.
func (b *RunBuilder) WithAttemptTimestamp(attemptTimestamp time.Time) *RunBuilder {
	b.run.AttemptTimestamp = attemptTimestamp
	return b
}

// WithRulesVersion specifies the rules version to use on the run.
func (b *RunBuilder) WithRulesVersion(value time.Time) *RunBuilder {
	b.run.RulesVersion = value
	return b
}

// WithRulesVersion specifies the config version to use on the run.
func (b *RunBuilder) WithConfigVersion(value time.Time) *RunBuilder {
	b.run.ConfigVersion = value
	return b
}

// WithAlgorithmsVersion specifies the algorithms version to use on the run.
func (b *RunBuilder) WithAlgorithmsVersion(value int64) *RunBuilder {
	b.run.AlgorithmsVersion = value
	return b
}

// WithShardCount specifies the number of shards to use on the run.
func (b *RunBuilder) WithShardCount(count int64) *RunBuilder {
	if b.progressSet {
		panic("Call WithShardCount before setting progress")
	}
	b.run.ShardCount = count
	b.run.Progress = count * 500
	return b
}

// WithNoProgress sets that no shards reported and no progress has been made.
func (b *RunBuilder) WithNoReportedProgress() *RunBuilder {
	b.run.ShardsReported = 0
	b.run.Progress = 0
	b.progressSet = true
	return b
}

// WithReportedProgress sets that all shards have reported, and some progress
// has been made. (progress is a value out of 1000 that is scaled to the
// number of shards).
func (b *RunBuilder) WithReportedProgress(progress int) *RunBuilder {
	b.run.ShardsReported = b.run.ShardCount
	b.run.Progress = b.run.ShardCount * int64(progress)
	b.progressSet = true
	return b
}

// WithCompletedProgress sets that all shards have reported, and the run
// has completed.
func (b *RunBuilder) WithCompletedProgress() *RunBuilder {
	b.run.ShardsReported = b.run.ShardCount
	b.run.Progress = b.run.ShardCount * 1000
	b.progressSet = true
	return b
}

func (b *RunBuilder) Build() *ReclusteringRun {
	return b.run
}

// SetRunsForTesting replaces the set of stored runs to match the given set.
func SetRunsForTesting(ctx context.Context, t testing.TB, rs []*ReclusteringRun) error {
	t.Helper()
	testutil.MustApply(ctx, t,
		spanner.Delete("ReclusteringRuns", spanner.AllKeys()))
	// Insert some ReclusteringRuns.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range rs {
			if err := Create(ctx, r); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}
