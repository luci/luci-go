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

package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/clustering/reclustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/state"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// Options represents parameters to the ingestion.
type Options struct {
	// The task index identifying the unique partition of the invocation
	// being ingested.
	TaskIndex int64
	// Project is the LUCI Project.
	Project string
	// PartitionTime is the start of the retention period of test results
	// being ingested.
	PartitionTime time.Time
	// Realm is the LUCI Realm of the test results.
	Realm string
	// InvocationID is the identity of the invocation being ingested.
	InvocationID string
	// The presubmit run (if any).
	PresubmitRun *PresubmitRun
	// The result of the build that was ingested.
	BuildStatus pb.BuildStatus
	// Whether the build was critical to the presubmit run.
	// Ignored if PresubmitRun is nil.
	BuildCritical bool
	// The gardener rotations the build is a part of. Corresponds to the
	// `sheriff_rotations` field of the build input properties.
	BuildGardenerRotations []string
	// Whether LUCI Analysis should preferentially ingest buganizer component
	// data instead of monorail, if both are available.
	// TODO(b/320356457): Delete once chromium buganizer migration complete.
	PreferBuganizerComponents bool
}

type PresubmitRun struct {
	// ID is the identity of the presubmit run (if any).
	ID *pb.PresubmitRunId
	// Owner is the owner of the presubmit
	// run (if any). This is the owner of the CL on which CQ+1/CQ+2 was
	// clicked (even in case of presubmit run with multiple CLs).
	Owner string
	// The mode of the presubmit run.
	// E.g. DRY_RUN, FULL_RUN, QUICK_DRY_RUN.
	Mode pb.PresubmitRunMode
	// The presubmit run's ending status.
	Status pb.PresubmitRunStatus
}

// ChunkStore is the interface for the blob store archiving chunks of test
// results for later re-clustering.
type ChunkStore interface {
	// Put saves the given chunk to storage. If successful, it returns
	// the randomly-assigned ID of the created object.
	Put(ctx context.Context, project string, content *cpb.Chunk) (string, error)
}

// ChunkSize is the number of test failures that are to appear in each chunk.
const ChunkSize = 1000

// Ingester handles the ingestion of test results for clustering.
type Ingester struct {
	chunkStore ChunkStore
	analysis   reclustering.Analysis
}

// New initialises a new Ingester.
func New(cs ChunkStore, a reclustering.Analysis) *Ingester {
	return &Ingester{
		chunkStore: cs,
		analysis:   a,
	}
}

// Ingestion handles the ingestion of a single invocation for clustering,
// in a streaming fashion.
type Ingestion struct {
	// ingestor provides access to shared objects for doing the ingestion.
	ingester *Ingester
	// opts is the Ingestion options.
	opts Options
	// buffer is the set of failures which have been queued for ingestion but
	// not yet written to chunks.
	//buffer []*cpb.Failure
	// chunkSeq is the number of the chunk failures written out.
	chunkSeq int
}

type TestVerdict struct {
	// Verdict is the ResultDB test verdict.
	Verdict *rdbpb.TestVariant
	// Sources are the code source(s) tested by the verdict. Note
	// that this may vary from verdict to verdict within an invocation.
	// May be unset if sources are unavailable.
	Sources *pb.Sources
	// TestVariantBranch captures statistics about the (test, variant, branch)
	// 3-tuple from which the verdict was obtained.
	// May be unset if sources are unavailable.
	TestVariantBranch *cpb.TestVariantBranch
}

// Ingest performs the ingestion of the specified test variants, with
// the specified options.
func (i *Ingester) Ingest(ctx context.Context, opts Options, tvs []TestVerdict) error {
	buffer := make([]*cpb.Failure, 0, ChunkSize)

	chunkSeq := 0
	writeChunk := func() error {
		if len(buffer) == 0 {
			panic("logic error: attempt to write empty chunk")
		}
		if len(buffer) > ChunkSize {
			panic("logic error: attempt to write oversize chunk")
		}
		// Copy failures buffer.
		failures := make([]*cpb.Failure, len(buffer))
		copy(failures, buffer)

		// Reset buffer.
		buffer = buffer[0:0]

		for i, f := range failures {
			f.ChunkIndex = int64(i + 1)
		}
		chunk := &cpb.Chunk{
			Failures: failures,
		}
		err := i.writeChunk(ctx, opts, chunkSeq, chunk)
		chunkSeq++
		return err
	}

	for _, tv := range tvs {
		failures := failuresFromTestVariant(opts, tv)
		// Write out chunks as needed, keeping all failures of
		// a test variant in one chunk, and the chunk size within
		// ChunkSize.
		if len(buffer)+len(failures) > ChunkSize {
			if err := writeChunk(); err != nil {
				return err
			}
		}
		buffer = append(buffer, failures...)
	}

	// Write out the last chunk (if needed).
	if len(buffer) > 0 {
		if err := writeChunk(); err != nil {
			return err
		}
	}
	return nil
}

// writeChunk will, for the given chunk:
// - Archive the failures to GCS.
// - Cluster the failures.
// - Write out the chunk clustering state.
// - Perform analysis.
func (i *Ingester) writeChunk(ctx context.Context, opts Options, chunkSeq int, chunk *cpb.Chunk) error {
	// Derive a chunkID deterministically from the ingested root invocation
	// ID, task index and chunk number. In case of retry this avoids ingesting
	// the same data twice.
	id := chunkID(opts.InvocationID, opts.TaskIndex, chunkSeq)

	_, err := state.Read(span.Single(ctx), opts.Project, id)
	if err == nil {
		// Chunk was already ingested as part of an earlier ingestion attempt.
		// Do not attempt to ingest again.
		return nil
	}
	if err != state.NotFoundErr {
		return err
	}

	// Upload the chunk. The objectID is randomly generated each time
	// so the actual insertion of the chunk will be atomic with the
	// ClusteringState row in Spanner.
	objectID, err := i.chunkStore.Put(ctx, opts.Project, chunk)
	if err != nil {
		return err
	}

	clusterState := &state.Entry{
		Project:       opts.Project,
		ChunkID:       id,
		PartitionTime: opts.PartitionTime,
		ObjectID:      objectID,
	}

	ruleset, err := reclustering.Ruleset(ctx, opts.Project, rules.StartingEpoch)
	if err != nil {
		return errors.Fmt("obtain ruleset: %w", err)
	}

	cfg, err := compiledcfg.Project(ctx, opts.Project, config.StartingEpoch)
	if err != nil {
		return errors.Fmt("obtain config: %w", err)
	}

	update, err := reclustering.PrepareUpdate(ctx, ruleset, cfg, chunk, clusterState)
	if err != nil {
		return err
	}

	updates := reclustering.NewPendingUpdates(ctx)
	updates.Add(update)
	if err := updates.Apply(ctx, i.analysis); err != nil {
		return err
	}
	return nil
}

// chunkID generates an identifier for the chunk deterministically.
// The identifier will be 32 lowercase hexadecimal characters. Generated
// identifiers will be approximately evenly distributed through
// the keyspace.
func chunkID(rootInvocationID string, taskIndex int64, chunkSeq int) string {
	content := fmt.Sprintf("%q:%v:%v", rootInvocationID, taskIndex, chunkSeq)
	sha256 := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sha256[:16])
}
