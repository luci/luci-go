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
	"testing"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuilder(t *testing.T) {
	Convey("Test Builder", t, func() {
		ctx := context.Background()

		pm, err := NewLocalPackageManager(t.TempDir())
		So(err, ShouldBeNil)

		b := NewBuilder(generators.Platforms{}, pm, actions.NewActionProcessor())
		b.SetExecutor(func(context.Context, *ExecutionConfig, *core.Derivation) error { return nil })

		Convey("ok", func() {
			pkg, err := b.Build(ctx, "", &Generator{
				Name: "first",
				Dependencies: []generators.Dependency{
					{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
				},
			})
			So(err, ShouldBeNil)

			// all packages are available
			So(func() { MustIncRefRecursiveRuntime(pkg) }, ShouldNotPanic)
			So(func() { MustDecRefRecursiveRuntime(pkg) }, ShouldNotPanic)
			So(pkg.Action.Name, ShouldEqual, "first")
		})

		Convey("preExec", func() {
			b.SetExecutor(func(context.Context, *ExecutionConfig, *core.Derivation) error { panic("unreachable") })
			b.SetPreExecuteHook(func(ctx context.Context, pkg actions.Package) error {
				return pkg.Handler.Build(func() error { return nil })
			})
		})

		var pkg actions.Package
		So(func() {
			pkg, err = b.Build(ctx, "", &Generator{
				Name: "first",
				Dependencies: []generators.Dependency{
					{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
				},
			})
		}, ShouldNotPanic)
		So(err, ShouldBeNil)

		// all packages are available
		So(func() { MustIncRefRecursiveRuntime(pkg) }, ShouldNotPanic)
		So(func() { MustDecRefRecursiveRuntime(pkg) }, ShouldNotPanic)
	})
}
