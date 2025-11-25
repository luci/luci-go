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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestSharding(t *testing.T) {
	t.Parallel()

	ftt.Run("AssignTestShardingAlgorithm", t, func(t *ftt.Test) {
		t.Run("Deterministic", func(t *ftt.Test) {
			id1 := ID("inv1")
			alg1, err := AssignTestShardingAlgorithm(id1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, alg1, should.NotBeEmpty)

			alg2, err := AssignTestShardingAlgorithm(id1)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, alg2, should.Equal(alg1))
		})
	})

	ftt.Run("ShardingAlgorithmByID", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			alg, err := ShardingAlgorithmByID("by_module")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, alg.ID(), should.Equal(TestShardingAlgorithmID("by_module")))
		})
		t.Run("Invalid", func(t *ftt.Test) {
			alg, err := ShardingAlgorithmByID("invalid")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, alg, should.BeNil)
		})
	})

	ftt.Run("Algorithms", t, func(t *ftt.Test) {
		t.Run("IDs are non-empty", func(t *ftt.Test) {
			for _, alg := range testShardingAlgorithms {
				assert.Loosely(t, alg.ID(), should.NotBeEmpty)
			}
		})
		t.Run("Total Weights add up to 100", func(t *ftt.Test) {
			totalWeight := 0
			for _, alg := range testShardingAlgorithms {
				totalWeight += alg.SamplingWeight()
			}
			assert.Loosely(t, totalWeight, should.Equal(100))
		})
		t.Run("ShardTestID", func(t *ftt.Test) {
			rootInvID := ID("inv1")
			testID := &pb.TestIdentifier{
				ModuleName:        "module",
				ModuleScheme:      "scheme",
				ModuleVariantHash: "hash",
				CoarseName:        "coarse",
				FineName:          "fine",
				CaseName:          "case",
			}

			for _, alg := range testShardingAlgorithms {
				t.Run(string(alg.ID()), func(t *ftt.Test) {
					shardIndex := alg.ShardTestID(rootInvID, testID)
					assert.Loosely(t, shardIndex, should.BeBetweenOrEqual(0, RootInvocationShardCount-1))
				})
			}
		})
	})
}
