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

package state

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering"
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/pbutil"
)

// Entry represents the clustering state of a chunk, consisting of:
//   - Metadata about what test results were clustered.
//   - Metadata about how the test results were clustered (the algorithms
//     and failure association rules used).
//   - The clusters each test result are in.
type Entry struct {
	// Project is the LUCI Project the chunk belongs to.
	Project string
	// ChunkID is the identity of the chunk of test results. 32 lowercase hexadecimal
	// characters assigned by the ingestion process.
	ChunkID string
	// PartitionTime is the start of the retention period of the test results in the chunk.
	PartitionTime time.Time
	// ObjectID is the identity of the object in GCS containing the chunk's test results.
	// 32 lowercase hexadecimal characters.
	ObjectID string
	// Clustering describes the latest clustering of test results in
	// the chunk.
	Clustering clustering.ClusterResults
	// LastUpdated is the Spanner commit time the row was last updated. Output only.
	LastUpdated time.Time
}

// NotFound is the error returned by Read if the row could not be found.
var NotFoundErr = errors.New("clustering state row not found")

// EndOfTable is the highest possible chunk ID that can be stored.
var EndOfTable = strings.Repeat("ff", 16)

// Create inserts clustering state for a chunk. Must be
// called in the context of a Spanner transaction.
func Create(ctx context.Context, e *Entry) error {
	if err := validateEntry(e); err != nil {
		return err
	}
	clusters, err := encodeClusters(e.Clustering.Algorithms, e.Clustering.Clusters)
	if err != nil {
		return err
	}
	ms := spanutil.InsertMap("ClusteringState", map[string]any{
		"Project":           e.Project,
		"ChunkID":           e.ChunkID,
		"PartitionTime":     e.PartitionTime,
		"ObjectID":          e.ObjectID,
		"AlgorithmsVersion": e.Clustering.AlgorithmsVersion,
		"ConfigVersion":     e.Clustering.ConfigVersion,
		"RulesVersion":      e.Clustering.RulesVersion,
		"Clusters":          clusters,
		"LastUpdated":       spanner.CommitTimestamp,
	})
	span.BufferWrite(ctx, ms)
	return nil
}

// ChunkKey represents the identify of a chunk.
type ChunkKey struct {
	Project string
	ChunkID string
}

// String returns a string representation of the key, for use in
// dictionaries.
func (k ChunkKey) String() string {
	return fmt.Sprintf("%q/%q", k.Project, k.ChunkID)
}

// ReadLastUpdated reads the last updated time of the specified chunks.
// If the chunk does not exist, the zero time value time.Time{} is returned.
// Unless an error is returned, the returned slice will be of the same length
// as chunkIDs. The i-th LastUpdated time returned will correspond
// to the i-th chunk ID requested.
func ReadLastUpdated(ctx context.Context, keys []ChunkKey) ([]time.Time, error) {
	var ks []spanner.Key
	for _, key := range keys {
		ks = append(ks, spanner.Key{key.Project, key.ChunkID})
	}

	results := make(map[string]time.Time)
	columns := []string{"Project", "ChunkID", "LastUpdated"}
	it := span.Read(ctx, "ClusteringState", spanner.KeySetFromKeys(ks...), columns)
	err := it.Do(func(r *spanner.Row) error {
		var project string
		var chunkID string
		var lastUpdated time.Time
		if err := r.Columns(&project, &chunkID, &lastUpdated); err != nil {
			return errors.Fmt("read clustering state row: %w", err)
		}
		key := ChunkKey{project, chunkID}
		results[key.String()] = lastUpdated
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]time.Time, len(keys))
	for i, key := range keys {
		// If an entry does not exist in results, this will set the
		// default value for *time.Time, which is nil.
		result[i] = results[key.String()]
	}
	return result, nil
}

// UpdateClustering updates the clustering results on a chunk.
//
// To avoid clobbering other concurrent updates, the caller should read
// the LastUpdated time of the chunk in the same transaction as it is
// updated (i.e. using ReadLastUpdated) and verify it matches the previous
// entry passed.
//
// The update uses the previous entry to avoid writing cluster data
// if it has not changed, which optimises the performance of minor
// reclusterings.
func UpdateClustering(ctx context.Context, previous *Entry, update *clustering.ClusterResults) error {
	if err := validateClusterResults(update); err != nil {
		return err
	}

	upd := make(map[string]any)
	upd["Project"] = previous.Project
	upd["ChunkID"] = previous.ChunkID
	upd["LastUpdated"] = spanner.CommitTimestamp
	upd["AlgorithmsVersion"] = update.AlgorithmsVersion
	upd["ConfigVersion"] = update.ConfigVersion
	upd["RulesVersion"] = update.RulesVersion

	if !clustering.AlgorithmsAndClustersEqual(&previous.Clustering, update) {
		// Clusters is a field that may be many kilobytes in size.
		// For efficiency, only write it to Spanner if it is changed.
		clusters, err := encodeClusters(update.Algorithms, update.Clusters)
		if err != nil {
			return err
		}
		upd["Clusters"] = clusters
	}

	span.BufferWrite(ctx, spanutil.UpdateMap("ClusteringState", upd))
	return nil
}

// Read reads clustering state for a chunk. Must be
// called in the context of a Spanner transaction. If no clustering
// state exists, the method returns the error NotFound.
func Read(ctx context.Context, project, chunkID string) (*Entry, error) {
	whereClause := "ChunkID = @chunkID"
	params := make(map[string]any)
	params["chunkID"] = chunkID

	limit := 1
	results, err := readWhere(ctx, project, whereClause, params, limit)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		// Row does not exist.
		return nil, NotFoundErr
	}
	return results[0], nil
}

// ReadNextOptions specifies options for ReadNextN.
type ReadNextOptions struct {
	// The exclusive lower bound of the range of ChunkIDs to read.
	// To read from the start of the table, leave this blank ("").
	StartChunkID string
	// The inclusive upper bound of the range of ChunkIDs to read.
	// To specify the end of the table, use the constant EndOfTable.
	EndChunkID string
	// The minimum AlgorithmsVersion that re-clustering wants to achieve.
	// If a row has an AlgorithmsVersion less than this value, it will
	// be eligble to be read.
	AlgorithmsVersion int64
	// The minimum ConfigVersion that re-clustering wants to achieve.
	// If a row has an RulesVersion less than this value, it will
	// be eligble to be read.
	ConfigVersion time.Time
	// The minimum RulesVersion that re-clustering wants to achieve.
	// If a row has an RulesVersion less than this value, it will
	// be eligble to be read.
	RulesVersion time.Time
}

// ReadNextN reads the n consecutively next clustering state entries
// matching ReadNextOptions.
func ReadNextN(ctx context.Context, project string, opts ReadNextOptions, n int) ([]*Entry, error) {
	params := make(map[string]any)
	whereClause := `
		ChunkId > @startChunkID AND ChunkId <= @endChunkID
		AND (AlgorithmsVersion < @algorithmsVersion
			OR ConfigVersion < @configVersion
			OR RulesVersion < @rulesVersion)
	`
	params["startChunkID"] = opts.StartChunkID
	params["endChunkID"] = opts.EndChunkID
	params["algorithmsVersion"] = opts.AlgorithmsVersion
	params["configVersion"] = opts.ConfigVersion
	params["rulesVersion"] = opts.RulesVersion

	return readWhere(ctx, project, whereClause, params, n)
}

func readWhere(ctx context.Context, project, whereClause string, params map[string]any, limit int) ([]*Entry, error) {
	stmt := spanner.NewStatement(`
		SELECT
		  ChunkId, PartitionTime, ObjectId,
		  AlgorithmsVersion,
		  ConfigVersion, RulesVersion,
		  LastUpdated, Clusters
		FROM ClusteringState
		WHERE Project = @project AND (` + whereClause + `)
		ORDER BY ChunkId
		LIMIT @limit
	`)
	for k, v := range params {
		stmt.Params[k] = v
	}
	stmt.Params["project"] = project
	stmt.Params["limit"] = limit

	it := span.Query(ctx, stmt)
	var b spanutil.Buffer
	results := []*Entry{}
	err := it.Do(func(r *spanner.Row) error {
		clusters := &cpb.ChunkClusters{}
		result := &Entry{Project: project}

		err := b.FromSpanner(r,
			&result.ChunkID, &result.PartitionTime, &result.ObjectID,
			&result.Clustering.AlgorithmsVersion,
			&result.Clustering.ConfigVersion, &result.Clustering.RulesVersion,
			&result.LastUpdated, clusters)
		if err != nil {
			return errors.Fmt("read clustering state row: %w", err)
		}

		result.Clustering.Algorithms, result.Clustering.Clusters, err = decodeClusters(clusters)
		if err != nil {
			return errors.Fmt("decode clusters: %w", err)
		}
		results = append(results, result)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// ReadProjects read all distinct projects with a clustering state entry..
func ReadProjects(ctx context.Context) ([]string, error) {
	stmt := spanner.NewStatement(`
		SELECT Project
		FROM ClusteringState
		GROUP BY Project
	`)
	it := span.Query(ctx, stmt)
	var projects []string
	err := it.Do(func(r *spanner.Row) error {
		var project string
		if err := r.Columns(&project); err != nil {
			return errors.Fmt("read project row: %w", err)
		}
		projects = append(projects, project)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return projects, nil
}

// EstimateChunks estimates the total number of chunks in the ClusteringState
// table for the given project.
func EstimateChunks(ctx context.Context, project string) (int, error) {
	stmt := spanner.NewStatement(`
	  SELECT ChunkId
	  FROM ClusteringState
	  WHERE Project = @project
	  ORDER BY ChunkId ASC
	  LIMIT 1 OFFSET 100
	`)
	stmt.Params["project"] = project

	it := span.Query(ctx, stmt)
	var chunkID string
	err := it.Do(func(r *spanner.Row) error {
		if err := r.Columns(&chunkID); err != nil {
			return errors.Fmt("read ChunkID row: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if chunkID == "" {
		// There was no 100th chunk ID. There must be less
		// than 100 chunks in the project.
		return 99, nil
	}
	return estimateChunksFromID(chunkID)
}

// estimateChunksFromID estimates the number of chunks in a project
// given the ID of the 100th chunk (in ascending keyspace order) in
// that project. The maximum estimate that will be returned is one
// billion. If there is no 100th chunk ID in the project, then
// there are clearly 99 chunks or less in the project.
func estimateChunksFromID(chunkID100 string) (int, error) {
	const MaxEstimate = 1000 * 1000 * 1000
	// This function uses the property that ChunkIDs are approximately
	// uniformly distributed. We use the following estimator of the
	// number of rows:
	//   100 / (fraction of keyspace used up to 100th row)
	// where fraction of keyspace used up to 100th row is:
	//   (ChunkID_100th + 1) / 2^128
	//
	// Where ChunkID_100th is the ChunkID of the 100th row (in keyspace
	// order), as a 128-bit integer (rather than hexadecimal string).
	//
	// Rearranging this estimator, we get:
	//   100 * 2^128 / (ChunkID_100th + 1)

	// numerator = 100 * 2 ^ 128
	var numerator big.Int
	numerator.Lsh(big.NewInt(100), 128)

	idBytes, err := hex.DecodeString(chunkID100)
	if err != nil {
		return 0, err
	}

	// denominator = ChunkID_100th + 1. We add one because
	// the keyspace consumed includes the ID itself.
	var denominator big.Int
	denominator.SetBytes(idBytes)
	denominator.Add(&denominator, big.NewInt(1))

	// estimate = numerator / denominator.
	var estimate big.Int
	estimate.Div(&numerator, &denominator)

	result := uint64(math.MaxUint64)
	if estimate.IsUint64() {
		result = estimate.Uint64()
	}
	if result > MaxEstimate {
		result = MaxEstimate
	}
	return int(result), nil
}

func validateEntry(e *Entry) error {
	if err := pbutil.ValidateProject(e.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
	switch {
	case !clustering.ChunkRe.MatchString(e.ChunkID):
		return fmt.Errorf("chunk ID %q is not valid", e.ChunkID)
	case e.PartitionTime.IsZero():
		return errors.New("partition time must be specified")
	case e.ObjectID == "":
		return errors.New("object ID must be specified")
	default:
		if err := validateClusterResults(&e.Clustering); err != nil {
			return err
		}
		return nil
	}
}

func validateClusterResults(c *clustering.ClusterResults) error {
	switch {
	case c.AlgorithmsVersion <= 0:
		return errors.New("algorithms version must be specified")
	case c.ConfigVersion.Before(config.StartingEpoch):
		return errors.New("config version must be valid")
	case c.RulesVersion.Before(rules.StartingEpoch):
		return errors.New("rules version must be valid")
	default:
		if err := validateAlgorithms(c.Algorithms); err != nil {
			return errors.Fmt("algorithms: %w", err)
		}
		if err := validateClusters(c.Clusters, c.Algorithms); err != nil {
			return errors.Fmt("clusters: %w", err)
		}
		return nil
	}
}

func validateAlgorithms(algorithms map[string]struct{}) error {
	for a := range algorithms {
		if !clustering.AlgorithmRe.MatchString(a) {
			return fmt.Errorf("algorithm %q is not valid", a)
		}
	}
	return nil
}

func validateClusters(clusters [][]clustering.ClusterID, algorithms map[string]struct{}) error {
	if len(clusters) == 0 {
		// Each chunk must have at least one test result, even
		// if that test result is in no clusters.
		return errors.New("there must be clustered test results in the chunk")
	}
	// Outer slice has on entry per test result.
	for i, tr := range clusters {
		// Inner slice has the list of clusters per test result.
		for j, c := range tr {
			if _, ok := algorithms[c.Algorithm]; !ok {
				return fmt.Errorf("test result %v: cluster %v: algorithm not in algorithms list: %q", i, j, c.Algorithm)
			}
			if err := c.ValidateIDPart(); err != nil {
				return errors.Fmt("test result %v: cluster %v: cluster ID is not valid: %w", i, j, err)
			}
		}
		if !clustering.ClustersAreSortedNoDuplicates(tr) {
			return fmt.Errorf("test result %v: clusters are not sorted, or there are duplicates: %v", i, tr)
		}
	}
	return nil
}
