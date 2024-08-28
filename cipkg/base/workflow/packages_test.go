// Copyright 2023 The LUCI Authors.
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

package workflow

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
)

func TestLocalPackageManager(t *testing.T) {
	ftt.Run("Test LocalPackageManager", t, func(t *ftt.Test) {
		pm, err := NewLocalPackageManager(t.TempDir())
		assert.Loosely(t, err, should.BeNil)

		t.Run("new package - ok", func(t *ftt.Test) {
			h := pm.Get("something")
			assert.Loosely(t, h, should.NotBeNil)
			err = h.IncRef()
			assert.Loosely(t, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)

			built := false
			err = h.Build(func() error {
				built = true
				return nil
			})
			assert.Loosely(t, built, should.BeTrue)
			assert.Loosely(t, err, should.BeNil)

			err = h.IncRef()
			assert.Loosely(t, err, should.BeNil)
			ok, err := h.TryRemove() // We can't remove a package in use.
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)

			err = h.DecRef()
			assert.Loosely(t, err, should.BeNil)
			ok, err = h.TryRemove()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)

			err = h.IncRef()
			assert.Loosely(t, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)
		})

		t.Run("new package - build error", func(t *ftt.Test) {
			h := pm.Get("something")
			assert.Loosely(t, h, should.NotBeNil)
			err = h.IncRef()
			assert.Loosely(t, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)

			someErr := errors.New("some err")
			built := false
			err = h.Build(func() error {
				built = true
				return someErr
			})
			assert.Loosely(t, built, should.BeTrue)
			assert.Loosely(t, errors.Is(err, someErr), should.BeTrue)
		})

		t.Run("lock", func(c *ftt.Test) {
			var wg sync.WaitGroup
			wg.Add(2)

			signal := make(chan struct{})

			go func() {
				h := pm.Get("something")
				err := h.Build(func() error {
					signal <- struct{}{} // holding exclusive lock
					<-signal             // waiting signal to release
					return nil
				})
				assert.Loosely(c, err, should.BeNil)
				wg.Done()
			}()

			<-signal
			var built atomic.Bool
			go func() {
				h := pm.Get("something")
				err := h.IncRef() // blocked by build
				// exclusive lock released
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, built.Load(), should.BeTrue)
				err = h.DecRef()
				assert.Loosely(c, err, should.BeNil)
				wg.Done()
			}()

			// We can't ensure `close` happened after IncRef. So this will be flaky
			// if something wrong here.
			runtime.Gosched() // Let IncRef execute.
			built.Store(true)
			close(signal)
			wg.Wait()
		})

		t.Run("prune", func(c *ftt.Test) {
			h := pm.Get("something")
			assert.Loosely(c, h, should.NotBeNil)
			err = h.IncRef()
			assert.Loosely(c, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)

			built := false
			err = h.Build(func() error {
				built = true
				return nil
			})
			assert.Loosely(c, built, should.BeTrue)
			assert.Loosely(c, err, should.BeNil)

			ctx := context.Background()
			err = h.IncRef()
			pm.Prune(ctx, 0, -1)
			ok, err := h.TryRemove() // We can't remove a package in use.
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, ok, should.BeFalse)

			err = h.DecRef()
			assert.Loosely(c, err, should.BeNil)
			pm.Prune(ctx, 0, -1) // Pruned
			err = h.IncRef()
			assert.Loosely(c, errors.Is(err, core.ErrPackageNotExist), should.BeTrue)
		})
	})
}

func TestRefRecursiveRuntime(t *testing.T) {
	ftt.Run("Test RefRecursiveRuntime", t, func(t *ftt.Test) {
		ctx := context.Background()

		pm, err := NewLocalPackageManager(t.TempDir())
		assert.Loosely(t, err, should.BeNil)

		b := NewBuilder(generators.Platforms{}, pm, actions.NewActionProcessor())
		pe := NewPackageExecutor("", nil, nil, nil, func(context.Context, *ExecutionConfig, *core.Derivation) error { return nil })
		first, err := b.Build(ctx, pe, &Generator{
			Name: "first",
			Dependencies: []generators.Dependency{
				{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run("ok", func(t *ftt.Test) {
			assert.Loosely(t, func() { MustIncRefRecursiveRuntime(first) }, should.NotPanic)
			assert.Loosely(t, func() { MustDecRefRecursiveRuntime(first) }, should.NotPanic)
		})
		t.Run("panic inc", func(t *ftt.Test) {
			assert.Loosely(t, func() { MustIncRefRecursiveRuntime(first) }, should.NotPanic)
			assert.Loosely(t, func() { MustIncRefRecursiveRuntime(first) }, should.Panic)
			assert.Loosely(t, func() { MustDecRefRecursiveRuntime(first) }, should.NotPanic)
		})
	})

}
