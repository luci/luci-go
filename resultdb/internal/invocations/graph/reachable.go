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
}

// ReachableInvocations is a set of reachable invocations,
// including summary information about each invocation.
// The set includes the root invocation(s) from which reachables were
// explored.
type ReachableInvocations map[invocations.ID]ReachableInvocation

func NewReachableInvocations() ReachableInvocations {
	return make(ReachableInvocations)
}

// Union adds other reachable invocations.
func (r ReachableInvocations) Union(other ReachableInvocations) {
	for id, invocation := range other {
		r[id] = invocation
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
func (r ReachableInvocations) IDSet() invocations.IDSet {
	result := make(invocations.IDSet, len(r))
	for id := range r {
		result[id] = struct{}{}
	}
	return result
}

// WithTestResultsIDSet returns the set of invocation IDs
// that contain test results.
func (r ReachableInvocations) WithTestResultsIDSet() invocations.IDSet {
	result := make(invocations.IDSet, len(r))
	for id, inv := range r {
		if inv.HasTestResults {
			result[id] = struct{}{}
		}
	}
	return result
}

// WithExonerationsIDSet returns the set of invocation IDs
// that contain test exonerations.
func (r ReachableInvocations) WithExonerationsIDSet() invocations.IDSet {
	result := make(invocations.IDSet, len(r))
	for id, inv := range r {
		if inv.HasTestExonerations {
			result[id] = struct{}{}
		}
	}
	return result
}

func (r ReachableInvocations) batches(size int) []ReachableInvocations {
	ids := r.IDSet().SortByRowID()
	batches := make([]ReachableInvocations, 0, 1+len(ids)/size)
	for len(ids) > 0 {
		batchSize := size
		if batchSize > len(ids) {
			batchSize = len(ids)
		}
		batch := make(ReachableInvocations, batchSize)
		for _, id := range ids[:batchSize] {
			batch[id] = r[id]
		}
		batches = append(batches, batch)
		ids = ids[batchSize:]
	}
	return batches
}

// marshal marshals the ReachableInvocations into a Redis value.
func (r ReachableInvocations) marshal() ([]byte, error) {
	if len(r) == 0 {
		return nil, errors.Reason("reachable invocations is invalid; at minimum the root invocation itself should be included").Err()
	}

	invocations := make([]*internalpb.ReachableInvocations_ReachableInvocation, 0, len(r))
	for id, inv := range r {
		invocations = append(invocations, &internalpb.ReachableInvocations_ReachableInvocation{
			InvocationId:        string(id),
			HasTestResults:      inv.HasTestResults,
			HasTestExonerations: inv.HasTestExonerations,
			Realm:               inv.Realm,
		})
	}
	message := &internalpb.ReachableInvocations{
		Invocations: invocations,
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
		return nil, err
	}

	message := &internalpb.ReachableInvocations{}
	if err := proto.Unmarshal(decompressed, message); err != nil {
		return nil, err
	}

	result := make(map[invocations.ID]ReachableInvocation, len(message.Invocations))
	for _, entry := range message.Invocations {
		result[invocations.ID(entry.InvocationId)] = ReachableInvocation{
			HasTestResults:      entry.HasTestResults,
			HasTestExonerations: entry.HasTestExonerations,
			Realm:               entry.Realm,
		}
	}
	return ReachableInvocations(result), nil
}
