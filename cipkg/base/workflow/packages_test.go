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

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLocalPackageManager(t *testing.T) {
	Convey("Test LocalPackageManager", t, func() {
		pm, err := NewLocalPackageManager(t.TempDir())
		So(err, ShouldBeNil)

		Convey("new package - ok", func() {
			h := pm.Get("something")
			So(h, ShouldNotBeNil)
			err = h.IncRef()
			So(errors.Is(err, core.ErrPackageNotExist), ShouldBeTrue)

			built := false
			err = h.Build(func() error {
				built = true
				return nil
			})
			So(built, ShouldBeTrue)
			So(err, ShouldBeNil)

			err = h.IncRef()
			So(err, ShouldBeNil)
			ok, err := h.TryRemove() // We can't remove a package in use.
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			err = h.DecRef()
			So(err, ShouldBeNil)
			ok, err = h.TryRemove()
			So(err, ShouldBeNil)
			So(ok, ShouldBeTrue)

			err = h.IncRef()
			So(errors.Is(err, core.ErrPackageNotExist), ShouldBeTrue)
		})

		Convey("new package - build error", func() {
			h := pm.Get("something")
			So(h, ShouldNotBeNil)
			err = h.IncRef()
			So(errors.Is(err, core.ErrPackageNotExist), ShouldBeTrue)

			someErr := errors.New("some err")
			built := false
			err = h.Build(func() error {
				built = true
				return someErr
			})
			So(built, ShouldBeTrue)
			So(errors.Is(err, someErr), ShouldBeTrue)
		})

		Convey("lock", func(c C) {
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
				c.So(err, ShouldBeNil)
				wg.Done()
			}()

			<-signal
			var built atomic.Bool
			go func() {
				h := pm.Get("something")
				err := h.IncRef() // blocked by build
				// exclusive lock released
				c.So(err, ShouldBeNil)
				c.So(built.Load(), ShouldBeTrue)
				err = h.DecRef()
				c.So(err, ShouldBeNil)
				wg.Done()
			}()

			// We can't ensure `close` happened after IncRef. So this will be flaky
			// if something wrong here.
			runtime.Gosched() // Let IncRef execute.
			built.Store(true)
			close(signal)
			wg.Wait()
		})

		Convey("prune", func() {
			h := pm.Get("something")
			So(h, ShouldNotBeNil)
			err = h.IncRef()
			So(errors.Is(err, core.ErrPackageNotExist), ShouldBeTrue)

			built := false
			err = h.Build(func() error {
				built = true
				return nil
			})
			So(built, ShouldBeTrue)
			So(err, ShouldBeNil)

			ctx := context.Background()
			err = h.IncRef()
			pm.Prune(ctx, 0, -1)
			ok, err := h.TryRemove() // We can't remove a package in use.
			So(err, ShouldBeNil)
			So(ok, ShouldBeFalse)

			err = h.DecRef()
			So(err, ShouldBeNil)
			pm.Prune(ctx, 0, -1) // Pruned
			err = h.IncRef()
			So(errors.Is(err, core.ErrPackageNotExist), ShouldBeTrue)
		})
	})
}

func TestRefRecursiveRuntime(t *testing.T) {
	Convey("Test RefRecursiveRuntime", t, func() {
		ctx := context.Background()

		pm, err := NewLocalPackageManager(t.TempDir())
		So(err, ShouldBeNil)

		b := NewBuilder(generators.Platforms{}, pm, actions.NewActionProcessor())
		pe := NewPackageExecutor("", nil, func(context.Context, *ExecutionConfig, *core.Derivation) error { return nil })
		first, err := b.Build(ctx, pe, &Generator{
			Name: "first",
			Dependencies: []generators.Dependency{
				{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
			},
		})
		So(err, ShouldBeNil)

		Convey("ok", func() {
			So(func() { MustIncRefRecursiveRuntime(first) }, ShouldNotPanic)
			So(func() { MustDecRefRecursiveRuntime(first) }, ShouldNotPanic)
		})
		Convey("panic inc", func() {
			So(func() { MustIncRefRecursiveRuntime(first) }, ShouldNotPanic)
			So(func() { MustIncRefRecursiveRuntime(first) }, ShouldPanic)
			So(func() { MustDecRefRecursiveRuntime(first) }, ShouldNotPanic)
		})
	})

}
