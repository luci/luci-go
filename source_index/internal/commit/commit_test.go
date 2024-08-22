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
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/testutil"
)

func TestCommit(t *testing.T) {
	ftt.Run("Commit", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		now := time.Date(2055, time.May, 5, 5, 5, 5, 5, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run("Save", func(t *ftt.Test) {
			commitToSave := Commit{
				key: Key{
					host:       "chromium.googlesource.com",
					repository: "chromium/src",
					commitHash: "50791c81152633c73485745d8311fafde0e4935a",
				},
				position: &Position{
					Ref:    "refs/heads/main",
					Number: 10,
				},
			}

			saveCommit := func() error {
				_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
					span.BufferWrite(ctx, commitToSave.Save())
					return nil
				})
				return err
			}

			t.Run("with position", func(t *ftt.Test) {
				err := saveCommit()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, MustReadAllForTesting(span.Single(ctx)), ShouldMatchCommits(([]Commit{commitToSave})))
			})

			t.Run("without position", func(t *ftt.Test) {
				commitToSave.position = nil

				err := saveCommit()

				assert.Loosely(t, err, should.BeNil)
				assert.That(t, MustReadAllForTesting(span.Single(ctx)), ShouldMatchCommits(([]Commit{commitToSave})))
			})
		})
	})
}
