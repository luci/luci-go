// Copyright 2024 The LUCI Authors.
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

package commit

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/testutil"
)

func TestKey(t *testing.T) {
	ftt.Run("Key", t, func(t *ftt.Test) {
		t.Run("NewKey", func(t *ftt.Test) {
			host := "chromium.googlesource.com"
			repository := "chromium/src"
			commitHash := "b66c0785db818311bf0ea486779f3326ba3ddf11"

			t.Run("valid", func(t *ftt.Test) {
				key, err := NewKey(host, repository, commitHash)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, key, ShouldMatchKey(Key{
					host:       host,
					repository: repository,
					commitHash: commitHash,
				}))
			})

			t.Run("invalid host", func(t *ftt.Test) {
				host = "chromium.invalidsource.com"

				key, err := NewKey(host, repository, commitHash)

				assert.That(t, err, should.ErrLike("invalid host"))
				assert.That(t, key, ShouldMatchKey(Key{}))
			})

			t.Run("invalid repository", func(t *ftt.Test) {
				repository = "chromium/"

				key, err := NewKey(host, repository, commitHash)

				assert.That(t, err, should.ErrLike("invalid repository"))
				assert.That(t, key, ShouldMatchKey(Key{}))
			})

			t.Run("invalid commit hash", func(t *ftt.Test) {
				commitHash = "not-a-hash"

				key, err := NewKey(host, repository, commitHash)

				assert.That(t, err, should.ErrLike("invalid commit hash"))
				assert.That(t, key, ShouldMatchKey(Key{}))
			})

			t.Run("mixed case commit hash", func(t *ftt.Test) {
				commitHash = "B66c0785Db818311bf0ea486779F3326ba3ddf11"

				key, err := NewKey(host, repository, commitHash)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, key, ShouldMatchKey(Key{
					host:       host,
					repository: repository,
					commitHash: "b66c0785db818311bf0ea486779f3326ba3ddf11",
				}))
			})
		})

		t.Run("spanner", func(t *ftt.Test) {
			ctx := testutil.SpannerTestContext(t)

			key := Key{
				host:       "chromium.googlesource.com",
				repository: "chromium/src",
				commitHash: "50791c81152633c73485745d8311fafde0e4935a",
			}
			commit := Commit{
				key: key,
				position: &Position{
					Ref:    "refs/heads/main",
					Number: 10,
				},
			}

			t.Run("Exits", func(t *ftt.Test) {
				t.Run("existing commit", func(t *ftt.Test) {
					MustSetForTesting(ctx, commit)

					exists, err := Exists(span.Single(ctx), key)

					assert.Loosely(t, err, should.BeNil)
					assert.That(t, exists, should.BeTrue)
				})

				t.Run("non-existing commit", func(t *ftt.Test) {
					commit.key.commitHash = "94f4b5c7c0bacc03caf215987a068db54b88af20"
					MustSetForTesting(ctx, commit)

					exists, err := Exists(span.Single(ctx), key)

					assert.Loosely(t, err, should.BeNil)
					assert.That(t, exists, should.BeFalse)
				})
			})
		})
	})
}
