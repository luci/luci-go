// Copyright 2020 The LUCI Authors.
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

package txndefer

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func ExampleFilterRDS() {
	ctx := FilterRDS(memory.Use(context.Background()))

	datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		Defer(ctx, func(context.Context) { fmt.Println("1") })
		Defer(ctx, func(context.Context) { fmt.Println("2") })
		return nil
	}, nil)

	// Output:
	// 2
	// 1
}

func TestFilter(t *testing.T) {
	t.Parallel()

	ftt.Run("With filter", t, func(t *ftt.Test) {
		ctx := FilterRDS(memory.Use(context.Background()))

		t.Run("Successful txn", func(t *ftt.Test) {
			ctx := context.WithValue(ctx, "123", "random extra value")
			called := false

			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				Defer(ctx, func(ctx context.Context) {
					assert.Loosely(t, datastore.CurrentTransaction(ctx), should.BeNil)
					assert.Loosely(t, ctx.Value("123"), should.Equal("random extra value"))
					called = true
				})
				return nil
			}, nil)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, called, should.BeTrue)
		})

		t.Run("Fatal txn error", func(t *ftt.Test) {
			called := false

			datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				Defer(ctx, func(context.Context) { called = true })
				return errors.New("boom")
			}, nil)

			assert.Loosely(t, called, should.BeFalse)
		})

		t.Run("Txn retries", func(t *ftt.Test) {
			attempt := 0
			calls := 0

			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				attempt++
				Defer(ctx, func(context.Context) { calls++ })
				if attempt < 3 {
					return datastore.ErrConcurrentTransaction
				}
				return nil
			}, nil)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, attempt, should.Equal(3))
			assert.Loosely(t, calls, should.Equal(1))
		})
	})
}
