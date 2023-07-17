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

// Package graph contains methods to explore reachable invocations.
package graph

import (
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/internal/invocations"
	internalpb "go.chromium.org/luci/resultdb/internal/proto"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ReachableInvocation contains summary information about a reachable
// invocation.
type ReachableInvocation struct {
	// HasTestResults stores whether the invocation has any test results.
	HasTestResults bool
	// HasTestResults stores whether the invocation has any test exonerations.
	HasTestExonerations bool
	// The realm of the invocation.
	Realm string
	// The source associated with the invocation, which can be looked up in
	// ReachableInvocations.Sources.
	// If no sources could be resolved, this is EmptySourceHash.
	SourceHash SourceHash
}

// ReachableInvocations is a set of reachable invocations,
// including summary information about each invocation.
// The set includes the root invocation(s) from which reachables were
// explored.
type ReachableInvocations struct {
	// The set of reachable invocations, including the root
	// invocation from which reachability was explored.
	Invocations map[invocations.ID]ReachableInvocation
	// The distinct code sources in the reachable invocation graph.
	// Stored here rather than on the invocations themselves to
	// simplify deduplicating sources objects as many will be the
	// same between invocations.
	Sources map[SourceHash]*pb.Sources
}

func NewReachableInvocations() ReachableInvocations {
	return ReachableInvocations{
		Invocations: make(map[invocations.ID]ReachableInvocation),
		Sources:     make(map[SourceHash]*pb.Sources),
	}
}

// Union adds other reachable invocations.
func (r *ReachableInvocations) Union(other ReachableInvocations) {
	for id, invocation := range other.Invocations {
		r.Invocations[id] = invocation
	}
	for id, sources := range other.Sources {
		r.Sources[id] = sources
	}
}

// Batches splits s into batches.
// The batches are sorted by RowID(), such that interval (minRowID, maxRowID)
// of each batch does not overlap with any other batch.
//
// The size of batch is hardcoded 50, because that's the maximum parallelism
// we get from Cloud Spanner.
func (r ReachableInvocations) Batches() []ReachableInvocations {
	return r.batches(50)
}

// IDSet returns the set of invocation IDs included in the list of
// reachable invocations.
func (r ReachableInvocations) IDSet() (invocations.IDSet, error) {
	// Yes, this is an artificial limit.  With 20,000 invocations you are already likely
	// to run into problems if you try to process all of these in one go (e.g. in a
	// Spanner query).  If you want more, use the batched call and handle a batch at a time.
	if len(r.Invocations) > MaxNodes {
		return nil, errors.Reason("more than %d invocations match", MaxNodes).Tag(TooManyTag).Err()
	}
	return r.idSetNoLimit(), nil
}

// IDSet returns the set of invocation IDs included in the list of
// reachable invocations.  This internal only version has no limit and
// is only for use where the limit will be checked in other ways.
func (r ReachableInvocations) idSetNoLimit() invocations.IDSet {
	result := make(invocations.IDSet, len(r.Invocations))
	for id := range r.Invocations {
		result[id] = struct{}{}
	}
	return result
}

// WithTestResultsIDSet returns the set of invocation IDs
// that contain test results.
func (r ReachableInvocations) WithTestResultsIDSet() (invocations.IDSet, error) {
	result := make(invocations.IDSet, len(r.Invocations))
	for id, inv := range r.Invocations {
		if inv.HasTestResults {
			result[id] = struct{}{}
		}
	}
	// Yes, this is an artificial limit.  With 20,000 invocations you are already likely
	// to run into problems if you try to process all of these in one go (e.g. in a
	// Spanner query).  If you want more, use the batched call and handle a batch at a time.
	if len(result) > MaxNodes {
		return nil, errors.Reason("more than %d invocations match", MaxNodes).Tag(TooManyTag).Err()
	}
	return result, nil
}

// WithExonerationsIDSet returns the set of invocation IDs
// that contain test exonerations.
func (r ReachableInvocations) WithExonerationsIDSet() (invocations.IDSet, error) {
	result := make(invocations.IDSet, len(r.Invocations))
	for id, inv := range r.Invocations {
		if inv.HasTestExonerations {
			result[id] = struct{}{}
		}
	}
	// Yes, this is an artificial limit.  With 20,000 invocations you are already likely
	// to run into problems if you try to process all of these in one go (e.g. in a
	// Spanner query).  If you want more, use the batched call and handle a batch at a time.
	if len(result) > MaxNodes {
		return nil, errors.Reason("more than %d invocations match", MaxNodes).Tag(TooManyTag).Err()
	}
	return result, nil
}

func (r ReachableInvocations) batches(size int) []ReachableInvocations {
	ids := r.idSetNoLimit().SortByRowID()
	batches := make([]ReachableInvocations, 0, 1+len(ids)/size)
	for len(ids) > 0 {
		batchSize := size
		if batchSize > len(ids) {
			batchSize = len(ids)
		}
		batch := NewReachableInvocations()
		for _, id := range ids[:batchSize] {
			inv := r.Invocations[id]
			batch.Invocations[id] = inv
			if inv.SourceHash != EmptySourceHash {
				batch.Sources[inv.SourceHash] = r.Sources[inv.SourceHash]
			}
		}
		batches = append(batches, batch)
		ids = ids[batchSize:]
	}
	return batches
}

// marshal marshals the ReachableInvocations into a Redis value.
func (r ReachableInvocations) marshal() ([]byte, error) {
	if len(r.Invocations) == 0 {
		return nil, errors.Reason("reachable invocations is invalid; at minimum the root invocation itself should be included").Err()
	}

	indexBySourceHash := make(map[SourceHash]int)
	distinctSources := make([]*pb.Sources, 0, len(r.Sources))
	for id, source := range r.Sources {
		distinctSources = append(distinctSources, source)
		indexBySourceHash[id] = len(distinctSources) - 1
	}

	invocations := make([]*internalpb.ReachableInvocations_ReachableInvocation, 0, len(r.Invocations))
	for id, inv := range r.Invocations {
		proto := &internalpb.ReachableInvocations_ReachableInvocation{
			InvocationId:        string(id),
			HasTestResults:      inv.HasTestResults,
			HasTestExonerations: inv.HasTestExonerations,
			Realm:               inv.Realm,
		}
		if inv.SourceHash != EmptySourceHash {
			proto.SourceOffset = int64(indexBySourceHash[inv.SourceHash]) + 1
		}
		invocations = append(invocations, proto)
	}
	message := &internalpb.ReachableInvocations{
		Invocations: invocations,
		Sources:     distinctSources,
	}
	result, err := proto.Marshal(message)
	if err != nil {
		return nil, err
	}
	return spanutil.Compress(result), nil
}

// unmarshalReachableInvocations unmarshals the ReachableInvocations from a Redis value.
func unmarshalReachableInvocations(value []byte) (ReachableInvocations, error) {
	// Assume 2x growth on decompression (will be resized as needed)
	decompressed := make([]byte, 0, len(value)*2)
	decompressed, err := spanutil.Decompress(value, decompressed)
	if err != nil {
		return ReachableInvocations{}, err
	}

	message := &internalpb.ReachableInvocations{}
	if err := proto.Unmarshal(decompressed, message); err != nil {
		return ReachableInvocations{}, err
	}

	sourceHashByIndex := make([]SourceHash, len(message.Sources))
	sources := make(map[SourceHash]*pb.Sources)
	for i, source := range message.Sources {
		hash := HashSources(source)
		sources[hash] = source
		sourceHashByIndex[i] = hash
	}

	invs := make(map[invocations.ID]ReachableInvocation, len(message.Invocations))
	for _, entry := range message.Invocations {
		inv := ReachableInvocation{
			HasTestResults:      entry.HasTestResults,
			HasTestExonerations: entry.HasTestExonerations,
			Realm:               entry.Realm,
			SourceHash:          EmptySourceHash,
		}
		if entry.SourceOffset > 0 {
			inv.SourceHash = sourceHashByIndex[entry.SourceOffset-1]
		}
		invs[invocations.ID(entry.InvocationId)] = inv
	}
	return ReachableInvocations{
		Invocations: invs,
		Sources:     sources,
	}, nil
}
