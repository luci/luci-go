// Copyright 2017 The LUCI Authors.
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

package engine

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestRunTxn(t *testing.T) {
	t.Parallel()

	ftt.Run("With mock context", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c = clock.Set(c, testclock.New(epoch))

		t.Run("Happy path", func(t *ftt.Test) {
			calls := 0
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				assert.Loosely(t, inner, should.BeNil)
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, calls, should.Equal(1)) // one successful attempt

			// Committed.
			job := Job{JobID: "123"}
			assert.Loosely(t, datastore.Get(c, &job), should.BeNil)
			assert.Loosely(t, job.Revision, should.Equal("abc"))
		})

		t.Run("Transient error", func(t *ftt.Test) {
			calls := 0
			transient := errors.New("transient error", transient.Tag)
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				assert.Loosely(t, inner, should.BeNil)
				return transient
			})
			assert.Loosely(t, err, should.Equal(transient))
			assert.Loosely(t, calls, should.Equal(defaultTransactionOptions.Attempts)) // all attempts

			// Not committed.
			job := Job{JobID: "123"}
			assert.Loosely(t, datastore.Get(c, &job), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("Fatal error", func(t *ftt.Test) {
			calls := 0
			fatal := errors.New("fatal error")
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				assert.Loosely(t, inner, should.BeNil)
				return fatal
			})
			assert.Loosely(t, err, should.Equal(fatal))
			assert.Loosely(t, calls, should.Equal(1)) // one failed attempt

			// Not committed.
			job := Job{JobID: "123"}
			assert.Loosely(t, datastore.Get(c, &job), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("Transient error, but marked as abortTransaction", func(t *ftt.Test) {
			calls := 0
			transient := errors.New("transient error", transient.Tag, abortTransaction)
			err := runTxn(c, func(ctx context.Context) error {
				calls++
				job := Job{JobID: "123", Revision: "abc"}
				inner := datastore.Put(ctx, &job)
				assert.Loosely(t, inner, should.BeNil)
				return transient
			})
			assert.Loosely(t, err, should.Equal(transient))
			assert.Loosely(t, calls, should.Equal(1)) // one failed attempt

			// Not committed.
			job := Job{JobID: "123"}
			assert.Loosely(t, datastore.Get(c, &job), should.Equal(datastore.ErrNoSuchEntity))
		})
	})
}

func TestOpsCache(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		calls := 0
		cb := func() error {
			calls++
			return nil
		}

		ops := opsCache{}
		assert.Loosely(t, ops.Do(c, "key", cb), should.BeNil)
		assert.Loosely(t, calls, should.Equal(1))

		// Second call is skipped.
		assert.Loosely(t, ops.Do(c, "key", cb), should.BeNil)
		assert.Loosely(t, calls, should.Equal(1))

		// Make sure memcache-based deduplication also works.
		ops.doneFlags = nil
		assert.Loosely(t, ops.Do(c, "key", cb), should.BeNil)
		assert.Loosely(t, calls, should.Equal(1))
	})
}
