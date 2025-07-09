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
)

func TestShardID(t *testing.T) {
	ftt.Run("RowID", t, func(t *ftt.Test) {
		assert.That(t, ShardID{RootInvocationID: "abc", ShardIndex: 0}.RowID(), should.Equal("0a7816bf:abc"))
		assert.That(t, ShardID{RootInvocationID: "abc", ShardIndex: 1}.RowID(), should.Equal("1a7816bf:abc"))
		assert.That(t, ShardID{RootInvocationID: "abc", ShardIndex: 2}.RowID(), should.Equal("2a7816bf:abc"))
		assert.That(t, ShardID{RootInvocationID: "abc", ShardIndex: 3}.RowID(), should.Equal("3a7816bf:abc"))
		assert.That(t, ShardID{RootInvocationID: "abc", ShardIndex: 10}.RowID(), should.Equal("aa7816bf:abc"))
		assert.That(t, ShardID{RootInvocationID: "abc", ShardIndex: 11}.RowID(), should.Equal("ba7816bf:abc"))
		assert.That(t, ShardID{RootInvocationID: "abc", ShardIndex: 15}.RowID(), should.Equal("fa7816bf:abc"))
	})
	ftt.Run("ShardIDFromRowID", t, func(t *ftt.Test) {
		assert.That(t, ShardIDFromRowID("0a7816bf:abc"), should.Equal(ShardID{RootInvocationID: "abc", ShardIndex: 0}))
		assert.That(t, ShardIDFromRowID("1a7816bf:abc"), should.Equal(ShardID{RootInvocationID: "abc", ShardIndex: 1}))
		assert.That(t, ShardIDFromRowID("2a7816bf:abc"), should.Equal(ShardID{RootInvocationID: "abc", ShardIndex: 2}))
		assert.That(t, ShardIDFromRowID("3a7816bf:abc"), should.Equal(ShardID{RootInvocationID: "abc", ShardIndex: 3}))
		assert.That(t, ShardIDFromRowID("aa7816bf:abc"), should.Equal(ShardID{RootInvocationID: "abc", ShardIndex: 10}))
		assert.That(t, ShardIDFromRowID("ba7816bf:abc"), should.Equal(ShardID{RootInvocationID: "abc", ShardIndex: 11}))
		assert.That(t, ShardIDFromRowID("fa7816bf:abc"), should.Equal(ShardID{RootInvocationID: "abc", ShardIndex: 15}))
	})
}
