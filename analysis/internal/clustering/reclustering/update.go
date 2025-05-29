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
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// TODO(crbug.com/1243174). Instrument the size of this cache so that we
// can monitor it.
var rulesCache = cache.NewRulesCache(caching.RegisterLRUCache[string, *cache.Ruleset](0))

// Ruleset returns the cached ruleset for the given project. If a minimum
// version of rule predicates is required, pass it as minimumPredicatesVersion.
// If a strong read is required, pass cache.StrongRead.
// Otherwise, pass rules.StartingEpoch.
func Ruleset(ctx context.Context, project string, minimumPredicatesVersion time.Time) (*cache.Ruleset, error) {
	ruleset, err := rulesCache.Ruleset(ctx, project, minimumPredicatesVersion)
	if err != nil {
		return nil, err
	}
	return ruleset, nil
}

// Analysis is the interface for cluster analysis.
type Analysis interface {
	// HandleUpdatedClusters handles (re-)clustered test results. It is called
	// after the spanner transaction effecting the (re-)clustering has
	// committed. commitTime is the Spanner time the transaction committed.
	HandleUpdatedClusters(ctx context.Context, updates *clustering.Update, commitTime time.Time) error
}

// PendingUpdate is a (re-)clustering of a chunk of test results
// that has not been applied to Spanner and/or sent for re-analysis
// yet.
type PendingUpdate struct {
	// Chunk is the identity of the chunk which will be updated.
	Chunk         state.ChunkKey
	existingState *state.Entry
	newClustering clustering.ClusterResults
	updates       []*clustering.FailureUpdate
}

// PrepareUpdate will (re-)cluster the specific chunk of test results,
// preparing an updated state for Spanner and updates to be exported
// to analysis. The caller can determine how to batch these updates/
// exports together, with help of the Size() method on the returned
// pending update.
//
// If the chunk does not exist in Spanner, pass a *state.Entry
// with project, chunkID, objectID and partitionTime set
// but with LastUpdated set to its zero value. The chunk will be
// clustered for the first time and saved to Spanner.
//
// If the chunk does exist in Spanner, pass the state.Entry read
// from Spanner, along with the test results. The chunk will be
// re-clustered and updated.
func PrepareUpdate(ctx context.Context, ruleset *cache.Ruleset, config *compiledcfg.ProjectConfig, chunk *cpb.Chunk, existingState *state.Entry) (upd *PendingUpdate, err error) {
	_, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/clustering/reclustering.PrepareUpdate",
		attribute.String("project", existingState.Project),
		attribute.String("chunkID", existingState.ChunkID),
	)
	defer func() { tracing.End(s, err) }()

	exists := !existingState.LastUpdated.IsZero()
	var existingClustering clustering.ClusterResults
	if !exists {
		existingClustering = algorithms.NewEmptyClusterResults(len(chunk.Failures))
	} else {
		if len(existingState.Clustering.Clusters) != len(chunk.Failures) {
			return nil, fmt.Errorf("existing clustering does not match chunk; got clusters for %v test results, want %v", len(existingClustering.Clusters), len(chunk.Failures))
		}
		existingClustering = existingState.Clustering
	}

	newClustering := algorithms.Cluster(config, ruleset, existingClustering, clustering.FailuresFromProtos(chunk.Failures))

	updates := prepareClusterUpdates(chunk, existingClustering, newClustering)

	return &PendingUpdate{
		Chunk:         state.ChunkKey{Project: existingState.Project, ChunkID: existingState.ChunkID},
		existingState: existingState,
		newClustering: newClustering,
		updates:       updates,
	}, nil
}

// Attempts to apply the update to Spanner.
//
// Important: Before calling this method, the caller should verify the chunks
// in Spanner still have the same LastUpdatedTime as passed to PrepareUpdate,
// in the same transaction as attempting this update.
// This will prevent clobbering a concurrently applied update or create.
//
// In case of an update race, PrepareUpdate should be retried with a more
// recent version of the chunk.
func (p *PendingUpdate) ApplyToSpanner(ctx context.Context) error {
	exists := !p.existingState.LastUpdated.IsZero()
	if !exists {
		clusterState := &state.Entry{
			Project:       p.existingState.Project,
			ChunkID:       p.existingState.ChunkID,
			PartitionTime: p.existingState.PartitionTime,
			ObjectID:      p.existingState.ObjectID,
			Clustering:    p.newClustering,
		}
		if err := state.Create(ctx, clusterState); err != nil {
			return err
		}
	} else {
		if err := state.UpdateClustering(ctx, p.existingState, &p.newClustering); err != nil {
			return err
		}
	}
	return nil
}

// ApplyToAnalysis exports changed failures for re-analysis. The
// Spanner commit time must be provided so that analysis has the
// correct update chronology.
func (p *PendingUpdate) ApplyToAnalysis(ctx context.Context, analysis Analysis, commitTime time.Time) error {
	if len(p.updates) > 0 {
		update := &clustering.Update{
			Project: p.existingState.Project,
			ChunkID: p.existingState.ChunkID,
			Updates: p.updates,
		}
		if err := analysis.HandleUpdatedClusters(ctx, update, commitTime); err != nil {
			return errors.Fmt("handle updated clusters (project: %s chunkID: %s): %w", p.existingState.Project, p.existingState.ChunkID, err)
		}
	}
	return nil
}

// EstimatedTransactionSize returns the estimated size of the
// Spanner transaction, in bytes.
func (p *PendingUpdate) EstimatedTransactionSize() int {
	if len(p.updates) > 0 {
		// This means we will be updating the clustering state in Spanner,
		// not just the Version fields.
		numClusters := 0
		for _, cs := range p.newClustering.Clusters {
			numClusters += len(cs)
		}
		// Est. 10 bytes per cluster, plus 200 bytes overhead.
		return 200 + numClusters*10
	}
	// The clustering state has not changed, only
	// AlgorithmsVersion and RulesVersion will be updated.
	return 200
}

// FailuresUpdated returns the number of failures that will
// exported for re-analysis as a result of the update.
func (p *PendingUpdate) FailuresUpdated() int {
	return len(p.updates)
}

func prepareClusterUpdates(chunk *cpb.Chunk, previousClustering clustering.ClusterResults, newClustering clustering.ClusterResults) []*clustering.FailureUpdate {
	var updates []*clustering.FailureUpdate
	for i, testResult := range chunk.Failures {
		previousClusters := previousClustering.Clusters[i]
		newClusters := newClustering.Clusters[i]

		if !clustering.ClustersEqual(previousClusters, newClusters) {
			update := &clustering.FailureUpdate{
				TestResult:       testResult,
				PreviousClusters: previousClusters,
				NewClusters:      newClusters,
			}
			updates = append(updates, update)
		}
	}
	return updates
}
