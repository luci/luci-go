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
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/spanutil"
	"go.chromium.org/luci/source_index/internal/testutil"
)

func TestCommit(t *testing.T) {
	ftt.Run("Commit", t, func(t *ftt.Test) {
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

		t.Run("Save", func(t *ftt.Test) {
			saveCommit := func() error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, commit.Save())
					return nil
				})
				return err
			}

			t.Run("with position", func(t *ftt.Test) {
				err := saveCommit()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, MustReadAllForTesting(span.Single(ctx)), ShouldMatchCommits(([]Commit{commit})))
			})

			t.Run("without position", func(t *ftt.Test) {
				commit.position = nil

				err := saveCommit()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, MustReadAllForTesting(span.Single(ctx)), ShouldMatchCommits(([]Commit{commit})))
			})
		})

		t.Run("ReadByKey", func(t *ftt.Test) {
			t.Run("with position", func(t *ftt.Test) {
				MustSetForTesting(ctx, commit)

				readCommit, err := ReadByKey(span.Single(ctx), key)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, readCommit, ShouldMatchCommitsIn(commit))
			})

			t.Run("without position", func(t *ftt.Test) {
				commit.position = nil
				MustSetForTesting(ctx, commit)

				readCommit, err := ReadByKey(span.Single(ctx), key)

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, readCommit, ShouldMatchCommitsIn(commit))
			})
		})

		t.Run("ReadByPosition", func(t *ftt.Test) {
			t.Run("with a matching commit", func(t *ftt.Test) {
				MustSetForTesting(ctx, commit)

				readCommit, err := ReadByPosition(span.Single(ctx), ReadByPositionOpts{
					Host:       commit.key.host,
					Repository: commit.key.repository,
					Position:   *commit.position,
				})

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, readCommit, ShouldMatchCommitsIn(commit))
			})

			t.Run("with no matching commits", func(t *ftt.Test) {
				MustSetForTesting(ctx, commit)

				readCommit, err := ReadByPosition(span.Single(ctx), ReadByPositionOpts{
					Host:       commit.key.host,
					Repository: commit.key.repository,
					Position: Position{
						Ref:    commit.position.Ref,
						Number: commit.position.Number + 1,
					},
				})

				assert.That(t, err, should.ErrLikeError(spanutil.ErrNotExists))
				assert.That(t, readCommit, ShouldMatchCommitsIn(Commit{}))
			})

			t.Run("with multiple matching commits", func(t *ftt.Test) {
				commitWithTheSamePosition := commit
				commitWithTheSamePosition.key.commitHash = "861fa1f90448586c6c1a7556e49d0933da741060"
				MustSetForTesting(ctx, commit, commitWithTheSamePosition)

				readCommit, err := ReadByPosition(span.Single(ctx), ReadByPositionOpts{
					Host:       commit.key.host,
					Repository: commit.key.repository,
					Position: Position{
						Ref:    commit.position.Ref,
						Number: commit.position.Number,
					},
				})

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, readCommit, ShouldMatchCommitsIn(commit, commitWithTheSamePosition))
			})
		})
	})
}
