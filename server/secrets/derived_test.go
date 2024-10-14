// Copyright 2019 The LUCI Authors.
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

package secrets

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDerivedStore(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Works", t, func(t *ftt.Test) {
		root := Secret{
			Active: []byte("1"),
			Passive: [][]byte{
				[]byte("2"),
				[]byte("3"),
			},
		}
		store := NewDerivedStore(root)

		s1, _ := store.RandomSecret(ctx, "secret_1")
		assert.Loosely(t, s1, should.Resemble(Secret{
			Active: derive([]byte("1"), "secret_1"),
			Passive: [][]byte{
				derive([]byte("2"), "secret_1"),
				derive([]byte("3"), "secret_1"),
			},
		}))

		// Test cache hit.
		s2, _ := store.RandomSecret(ctx, "secret_1")
		assert.Loosely(t, s2, should.Resemble(s1))

		// Test noop root key change.
		store.SetRoot(root)
		s2, _ = store.RandomSecret(ctx, "secret_1")
		assert.Loosely(t, s2, should.Resemble(s1))

		// Test actual root key change.
		store.SetRoot(Secret{Active: []byte("zzz")})
		s2, _ = store.RandomSecret(ctx, "secret_1")
		assert.Loosely(t, s2, should.NotResemble(s1))
	})
}
