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
	"encoding/hex"
	"fmt"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/clustering"
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
)

// decodeClusters decodes:
// - the set of algorithms used for clustering, and
// - the clusters assigned to each test result
// from the protobuf representation.
func decodeClusters(cc *cpb.ChunkClusters) (map[string]struct{}, [][]clustering.ClusterID, error) {
	if cc == nil {
		return nil, nil, errors.New("proto must be specified")
	}
	typeCount := int64(len(cc.ClusterTypes))
	clusterCount := int64(len(cc.ReferencedClusters))

	algorithms := make(map[string]struct{})
	for _, ct := range cc.ClusterTypes {
		algorithms[ct.Algorithm] = struct{}{}
	}

	clusterIDs := make([][]clustering.ClusterID, len(cc.ResultClusters))
	for i, rc := range cc.ResultClusters {
		// For each test result.
		clusters := make([]clustering.ClusterID, len(rc.ClusterRefs))
		for j, ref := range rc.ClusterRefs {
			// Decode each reference to a cluster ID.
			if ref < 0 || ref >= clusterCount {
				return nil, nil, fmt.Errorf("reference to non-existent cluster (%v) from result %v; only %v referenced clusters defined", ref, i, clusterCount)
			}
			cluster := cc.ReferencedClusters[ref]
			if cluster.TypeRef < 0 || cluster.TypeRef >= typeCount {
				return nil, nil, fmt.Errorf("reference to non-existent type (%v) from referenced cluster %v; only %v types defined", cluster.TypeRef, ref, typeCount)
			}
			t := cc.ClusterTypes[cluster.TypeRef]
			clusters[j] = clustering.ClusterID{
				Algorithm: t.Algorithm,
				ID:        hex.EncodeToString(cluster.ClusterId),
			}
		}
		clusterIDs[i] = clusters
	}
	return algorithms, clusterIDs, nil
}

// encodeClusters encodes:
// - the set of algorithms used for clustering, and
// - the clusters assigned to each test result
// to the protobuf representation.
func encodeClusters(algorithms map[string]struct{}, clusterIDs [][]clustering.ClusterID) (*cpb.ChunkClusters, error) {
	rb := newRefBuilder()
	for a := range algorithms {
		rb.registerClusterType(a)
	}

	resultClusters := make([]*cpb.TestResultClusters, len(clusterIDs))
	for i, ids := range clusterIDs {
		clusters := &cpb.TestResultClusters{}
		clusters.ClusterRefs = make([]int64, len(ids))
		for j, id := range ids {
			clusterRef, err := rb.referenceCluster(id)
			if err != nil {
				return nil, errors.Annotate(err, "cluster ID %s/%s is invalid", id.Algorithm, id.ID).Err()
			}
			clusters.ClusterRefs[j] = clusterRef
		}
		resultClusters[i] = clusters
	}
	result := &cpb.ChunkClusters{
		ClusterTypes:       rb.types,
		ReferencedClusters: rb.refs,
		ResultClusters:     resultClusters,
	}
	return result, nil
}

// refBuilder assists in constructing the type and cluster references used in
// the proto representation.
type refBuilder struct {
	types []*cpb.ClusterType
	// typeMap is a mapping from algorithm name to the index in types.
	typeMap map[string]int
	refs    []*cpb.ReferencedCluster
	// refMap is a mapping from (algorithm name, cluster ID) to the
	// the corresponding cluster reference in refs.
	refMap map[string]int
}

func newRefBuilder() *refBuilder {
	return &refBuilder{
		typeMap: make(map[string]int),
		refMap:  make(map[string]int),
	}
}

func (rb *refBuilder) referenceCluster(ref clustering.ClusterID) (int64, error) {
	refKey := ref.Key()
	idx, ok := rb.refMap[refKey]
	if !ok {
		// Convert from hexadecimal to byte representation, for storage
		// efficiency.
		id, err := hex.DecodeString(ref.ID)
		if err != nil {
			return -1, err
		}
		typeRef, err := rb.referenceClusterType(ref.Algorithm)
		if err != nil {
			return -1, err
		}
		ref := &cpb.ReferencedCluster{
			TypeRef:   typeRef,
			ClusterId: id,
		}
		idx = len(rb.refs)
		rb.refMap[refKey] = idx
		rb.refs = append(rb.refs, ref)
	}
	return int64(idx), nil
}

func (rb *refBuilder) referenceClusterType(algorithm string) (int64, error) {
	idx, ok := rb.typeMap[algorithm]
	if !ok {
		return -1, fmt.Errorf("a test result was clustered with an unregistered algorithm: %s", algorithm)
	}
	return int64(idx), nil
}

func (rb *refBuilder) registerClusterType(algorithm string) {
	idx := len(rb.types)
	rb.types = append(rb.types, &cpb.ClusterType{Algorithm: algorithm})
	rb.typeMap[algorithm] = idx
}
