// Copyright 2016 The LUCI Authors.
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

package shards

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestShards(t *testing.T) {
	ftt.Run("Build shards, serialize and deserialize", t, func(t *ftt.Test) {
		// Build sharded set of 500 strings.
		set := make(Set, 10)
		for i := 0; i < 500; i++ {
			set.Insert([]byte(fmt.Sprintf("blob #%d", i)))
		}

		// Read same strings. Should be noop, they are already in the set.
		for i := 0; i < 500; i++ {
			set.Insert([]byte(fmt.Sprintf("blob #%d", i)))
		}

		// Make sure shards are balanced, and all data is there.
		totalLen := 0
		for _, shard := range set {
			assert.Loosely(t, len(shard), should.BeGreaterThan(45))
			assert.Loosely(t, len(shard), should.BeLessThan(55))
			totalLen += len(shard)
		}
		assert.Loosely(t, totalLen, should.Equal(500))

		// Serialize and deserialize first shard, should be left unchanged.
		shard, err := ParseShard(set[0].Serialize())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, shard, should.Match(set[0]))
	})

	ftt.Run("Empty shards serialization", t, func(t *ftt.Test) {
		var shard Shard
		deserialized, err := ParseShard(shard.Serialize())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(deserialized), should.BeZero)
	})

	ftt.Run("Deserialize garbage", t, func(t *ftt.Test) {
		_, err := ParseShard([]byte("blah"))
		assert.Loosely(t, err, should.NotBeNil)
	})
}
