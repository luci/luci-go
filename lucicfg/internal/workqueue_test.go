// Copyright 2025 The LUCI Authors.
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

package internal

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWorkQueue(t *testing.T) {
	t.Parallel()

	t.Run("OK", func(t *testing.T) {
		var calls atomic.Int32

		wq, _ := NewWorkQueue[int](context.Background())
		wq.Launch(func(work int) error {
			calls.Add(1)
			if work != 99 {
				wq.Submit(work + 1)
			}
			return nil
		})
		wq.Submit(0)
		assert.NoErr(t, wq.Wait())
		assert.That(t, calls.Load(), should.Equal(int32(100)))
	})

	t.Run("Fail", func(t *testing.T) {
		var calls atomic.Int32

		err := errors.New("boom")

		ctxErr := make(chan error, 1)

		wq, ctx := NewWorkQueue[int](context.Background())
		wq.Launch(func(work int) error {
			calls.Add(1)
			if work == 49 {
				return err
			}
			wq.Submit(work + 1)
			if work == 0 {
				<-ctx.Done()
				ctxErr <- ctx.Err()
				return nil
			}
			return nil
		})

		wq.Submit(0)
		assert.That(t, wq.Wait(), should.Equal(err))
		assert.That(t, calls.Load(), should.Equal(int32(50)))
		assert.That(t, errors.Is(<-ctxErr, context.Canceled), should.BeTrue)
	})
}
