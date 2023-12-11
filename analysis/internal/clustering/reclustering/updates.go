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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/tracing"
)

const (
	// Maximum size of pending Spanner transactions, in bytes. Once
	// a transaction meets or exceeds this size, it will be committed.
	maxTransactionBytes = 1000 * 1000
	// Maximum number of failures to export to analysis at a time.
	// Once the failures to be exported meets or exceeds this size, they will
	// be committed.
	maxAnalysisSize = 1000
	// Maximum amount of time between Spanner commits. If no updates have
	// been committed to Spanner for this time, a commit will be made at
	// the next earliest opportunity. This limits how much work can be
	// lost in case of an error or an update conflict.
	maxPendingTime = 2 * time.Second
)

// UpdateRaceErr is the error returned by UpdateClustering if a concurrent
// modification (or deletion) of a chunk is detected.
var UpdateRaceErr = errors.New("concurrent modification to cluster")

// PendingUpdates represents a pending set of chunk updates. It facilitates
// batching updates together for efficiency.
type PendingUpdates struct {
	updates                 []*PendingUpdate
	pendingTransactionBytes int
	pendingAnalysisSize     int
	lastCommit              time.Time
}

// NewPendingUpdates initialises a new PendingUpdates.
func NewPendingUpdates(ctx context.Context) *PendingUpdates {
	return &PendingUpdates{
		updates:                 nil,
		pendingTransactionBytes: 0,
		pendingAnalysisSize:     0,
		lastCommit:              clock.Now(ctx),
	}
}

// Add adds the specified update to the set of pending updates.
func (p *PendingUpdates) Add(update *PendingUpdate) {
	p.updates = append(p.updates, update)
	p.pendingTransactionBytes += update.EstimatedTransactionSize()
	p.pendingAnalysisSize += update.FailuresUpdated()
}

// ShouldApply returns whether the updates should be applied now because
// they have reached a maximum size or time limit.
func (p *PendingUpdates) ShouldApply(ctx context.Context) bool {
	return p.pendingTransactionBytes > maxTransactionBytes ||
		p.pendingAnalysisSize > maxAnalysisSize ||
		clock.Now(ctx).Sub(p.lastCommit) > maxPendingTime
}

// Apply applies the chunk updates to Spanner and exports them for re-analysis.
// If some applications failed because of a concurrent modification, the method
// returns UpdateRaceErr. In this case, the caller should construct the updates
// again from a fresh read of the Clustering State and retry.
// Note that some of the updates may have successfully applied.
func (p *PendingUpdates) Apply(ctx context.Context, analysis Analysis) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/clustering/reclustering.Apply")
	defer func() { tracing.End(s, err) }()

	var appliedUpdates []*PendingUpdate
	f := func(ctx context.Context) error {
		var keys []state.ChunkKey
		for _, pu := range p.updates {
			keys = append(keys, pu.Chunk)
		}
		lastUpdated, err := state.ReadLastUpdated(ctx, keys)
		if err != nil {
			return errors.Annotate(err, "read last updated").Err()
		}

		appliedUpdates = nil
		for i, pu := range p.updates {
			actualLastUpdated := lastUpdated[i]
			expectedLastUpdated := pu.existingState.LastUpdated
			if !expectedLastUpdated.Equal(actualLastUpdated) {
				// Our update raced with another update.
				continue
			}
			if err := pu.ApplyToSpanner(ctx); err != nil {
				return errors.Annotate(err, "apply to spanner").Err()
			}
			appliedUpdates = append(appliedUpdates, pu)
		}
		return nil
	}
	commitTime, err := span.ReadWriteTransaction(ctx, f)
	if err != nil {
		return err
	}
	for _, pu := range appliedUpdates {
		if err := pu.ApplyToAnalysis(ctx, analysis, commitTime); err != nil {
			return errors.Annotate(err, "export analysis").Err()
		}
	}
	if len(appliedUpdates) != len(p.updates) {
		// One more more updates raced.
		return UpdateRaceErr
	}
	return nil
}
