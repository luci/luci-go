// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rootinvocations

import (
	"crypto/sha256"
	"encoding/binary"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Sharding algorithms try to optimise for two (sometimes competing) objectives:
// - Minimise the number of shards written to in any given BatchCreateTestResults request,
//   to minimise the number of Spanner participants in any Read/Write transaction and
//   thereby optimise write throughput.
// - For the root invocation as a whole, distribute tests evenly between shards,
//   to optimise aggregation performance (read throughput) and to avoid bottlenecking
//   writes on a small number of span servers (write throughput), especially for
//   very large concurrently uploaded invocations where the throughput of a single
//   span server is a problem.

// TestShardingAlgorithmID identifies a test sharding algorithm.
type TestShardingAlgorithmID string

// The set of known sharding algorithms.
//
// When rolling out new sharding algorithms, make sure that *all* Recorder instances
// (i.e. both stable and canary) know how to implement the new algorithm before
// using creating RootInvocations that reference them.
var testShardingAlgorithms = []ShardingAlgorithm{
	&ByModuleShardingAlgorithm{},
	&ByCaseNameShardingAlgorithm{},
	&ByFineNameHybridShardingAlgorithm{},
	&ByFineNameShardingAlgorithm{},
}

// AssignTestShardingAlgorithm assigns a test sharding algorithm to a root invocation.
func AssignTestShardingAlgorithm(rootInvocationID ID) (TestShardingAlgorithmID, error) {
	var totalSamplingWeight int
	for _, alg := range testShardingAlgorithms {
		totalSamplingWeight += alg.SamplingWeight()
	}
	if totalSamplingWeight == 0 {
		return "", errors.New("no sharding algorithms are enabled")
	}

	// For determinism, do not use a random number generator, but
	// hash the root invocation ID as the seed.
	h := sha256.Sum256([]byte(string(rootInvocationID)))
	// Take the first 8 bytes as a uint64.
	val := binary.BigEndian.Uint64(h[:8])

	// Obtain a value in the range [0, totalSamplingWeight).
	selected := int(val % uint64(totalSamplingWeight))

	for _, alg := range testShardingAlgorithms {
		selected -= alg.SamplingWeight()
		if selected < 0 {
			return alg.ID(), nil
		}
	}
	return "", errors.New("logic error assigning sharding algorithm")
}

// ShardingAlgorithmByID returns the sharding algorithm with the given ID.
func ShardingAlgorithmByID(id TestShardingAlgorithmID) (ShardingAlgorithm, error) {
	for _, alg := range testShardingAlgorithms {
		if alg.ID() == id {
			return alg, nil
		}
	}
	return nil, errors.Fmt("unknown test sharding algorithm %q; is this backend running up-to-date code?", id)
}

// ShardingAlgorithm is an interface for test sharding algorithms.
type ShardingAlgorithm interface {
	// ID returns the ID of the sharding algorithm.
	ID() TestShardingAlgorithmID
	// SamplingWeight returns a sampling weight (0-100) for this algorithm.
	// The higher the weight, the more likely the algorithm will be used for a new
	// root invocation. A value of zero means the algorithm shall not be used for
	// new root invocations.
	SamplingWeight() int
	// ShardTestID assigns a test identifier in a root invocation to a shard.
	//
	// This method must be deterministic; query code assumes that the same test identifier
	// in the same root invocation will always be assigned to the same shard.
	ShardTestID(rootInvocationID ID, testID *pb.TestIdentifier) int
}

// ByModuleShardingAlgorithm assigns test results based on the module name.
type ByModuleShardingAlgorithm struct{}

// ID implements ShardingAlgorithm.ID.
func (ByModuleShardingAlgorithm) ID() TestShardingAlgorithmID {
	return "by_module"
}

// SamplingWeight implements ShardingAlgorithm.SamplingWeight.
func (ByModuleShardingAlgorithm) SamplingWeight() int {
	return 25
}

// ShardTestID implements ShardingAlgorithm.ShardTestID.
func (ByModuleShardingAlgorithm) ShardTestID(rootInvocationID ID, testID *pb.TestIdentifier) int {
	// Assign each test result to its own shard based on a hash of the
	// module name and root invocation.
	// We include the root invocation to avoid any skew in the distribution
	// of test results across shards (e.g. from some modules always having
	// more results than others).
	const delimeter = "\x00"
	input := string(rootInvocationID) + delimeter +
		testID.ModuleName + delimeter +
		testID.ModuleScheme + delimeter +
		testID.ModuleVariantHash

	h := sha256.Sum256([]byte(input))
	// Take the first 8 bytes as a uint64.
	val := binary.BigEndian.Uint64(h[:8])
	return int(val % RootInvocationShardCount)
}

// ByCaseNameShardingAlgorithm assigns test results based on the full test identifier.
type ByCaseNameShardingAlgorithm struct{}

// ID implements ShardingAlgorithm.ID.
func (ByCaseNameShardingAlgorithm) ID() TestShardingAlgorithmID {
	return "by_case_name"
}

// SamplingWeight implements ShardingAlgorithm.SamplingWeight.
func (ByCaseNameShardingAlgorithm) SamplingWeight() int {
	return 25
}

// ShardTestID implements ShardingAlgorithm.ShardTestID.
func (ByCaseNameShardingAlgorithm) ShardTestID(rootInvocationID ID, testID *pb.TestIdentifier) int {
	// Assign each test result to its own shard based on a hash of the
	// fully-qualified test identifier and root invocation.
	// We include the root invocation to avoid any skew in the distribution
	// of test results across shards (e.g. from some test IDs being
	// more popular when viewed across all root invocations).
	const delimeter = "\x00"
	input := string(rootInvocationID) + delimeter +
		testID.ModuleName + delimeter +
		testID.ModuleScheme + delimeter +
		testID.ModuleVariantHash + delimeter +
		testID.CoarseName + delimeter +
		testID.FineName + delimeter +
		testID.CaseName

	h := sha256.Sum256([]byte(input))
	// Take the first 8 bytes as a uint64.
	val := binary.BigEndian.Uint64(h[:8])
	return int(val % RootInvocationShardCount)
}

// ByFineNameShardingAlgorithm assigns test results based on the fine name.
type ByFineNameShardingAlgorithm struct{}

// ID implements ShardingAlgorithm.ID.
func (ByFineNameShardingAlgorithm) ID() TestShardingAlgorithmID {
	return "by_fine_name"
}

// SamplingWeight implements ShardingAlgorithm.SamplingWeight.
func (ByFineNameShardingAlgorithm) SamplingWeight() int {
	return 25
}

// ShardTestID implements ShardingAlgorithm.ShardTestID.
func (ByFineNameShardingAlgorithm) ShardTestID(rootInvocationID ID, testID *pb.TestIdentifier) int {
	// Assign each test result to its own shard based on a hash of the
	// fully-qualified fine name and root invocation.
	// We include the root invocation to avoid any skew in the distribution
	// of test results across shards (e.g. from some fine names always having
	// more results than others).
	const delimeter = "\x00"
	input := string(rootInvocationID) + delimeter +
		testID.ModuleName + delimeter +
		testID.ModuleScheme + delimeter +
		testID.ModuleVariantHash + delimeter +
		testID.CoarseName + delimeter +
		testID.FineName

	h := sha256.Sum256([]byte(input))
	// Take the first 8 bytes as a uint64.
	val := binary.BigEndian.Uint64(h[:8])
	return int(val % RootInvocationShardCount)
}

// ByFineNameHybridShardingAlgorithm assigns test results based on the fine name if available,
// otherwise the case name.
type ByFineNameHybridShardingAlgorithm struct{}

// ID implements ShardingAlgorithm.ID.
func (ByFineNameHybridShardingAlgorithm) ID() TestShardingAlgorithmID {
	return "by_fine_name_hybrid"
}

// SamplingWeight implements ShardingAlgorithm.SamplingWeight.
func (ByFineNameHybridShardingAlgorithm) SamplingWeight() int {
	return 25
}

// ShardTestID implements ShardingAlgorithm.ShardTestID.
func (ByFineNameHybridShardingAlgorithm) ShardTestID(rootInvocationID ID, testID *pb.TestIdentifier) int {
	if testID.FineName == "" {
		// If there is no fine name (e.g. results mostly using a flat scheme
		// like legacy), use the case name sharding algorithm.
		// This avoids pooling too many results in to the same shard.
		return ByCaseNameShardingAlgorithm{}.ShardTestID(rootInvocationID, testID)
	}
	return ByFineNameShardingAlgorithm{}.ShardTestID(rootInvocationID, testID)
}
