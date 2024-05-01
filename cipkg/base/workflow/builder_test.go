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
	"fmt"
	"os"
	"testing"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/common/exec/execmock"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExecutor(t *testing.T) {
	Convey("Test executor", t, func() {
		ctx := execmock.Init(context.Background())
		uses := execmock.Simple.Mock(ctx, execmock.SimpleInput{})
		out := t.TempDir()

		Convey("ok", func() {
			err := Execute(ctx, &ExecutionConfig{
				OutputDir:  out,
				WorkingDir: out,
				Stdin:      os.Stdin,
				Stdout:     os.Stdout,
				Stderr:     os.Stderr,
			}, &core.Derivation{
				Args: []string{"bin", "-args", "1"},
				Env:  []string{"env1=1", "env2=2"},
			})
			So(err, ShouldBeNil)
			usage := uses.Snapshot()
			So(usage[0].Args, ShouldEqual, []string{"bin", "-args", "1"})
			So(usage[0].Env.Sorted(), ShouldEqual, []string{"env1=1", "env2=2", fmt.Sprintf("out=%s", out)})
		})

		Convey("env - override out", func() {
			err := Execute(ctx, &ExecutionConfig{
				OutputDir:  out,
				WorkingDir: out,
				Stdin:      os.Stdin,
				Stdout:     os.Stdout,
				Stderr:     os.Stderr,
			}, &core.Derivation{
				Args: []string{"bin", "-args", "1"},
				Env:  []string{"env1=1", "env2=2", "out=something"},
			})
			So(err, ShouldBeNil)
			usage := uses.Snapshot()
			So(usage[0].Args, ShouldEqual, []string{"bin", "-args", "1"})
			So(usage[0].Env.Sorted(), ShouldEqual, []string{"env1=1", "env2=2", fmt.Sprintf("out=%s", out)})
		})
	})
}

func TestBuilder(t *testing.T) {
	Convey("Test Builder", t, func() {
		ctx := context.Background()

		pm, err := NewLocalPackageManager(t.TempDir())
		So(err, ShouldBeNil)

		b := NewBuilder(generators.Platforms{}, pm, actions.NewActionProcessor())
		pe := NewPackageExecutor("", nil, nil, nil, func(context.Context, *ExecutionConfig, *core.Derivation) error { return nil })

		Convey("ok", func() {
			pkg, err := b.Build(ctx, pe, &Generator{
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

		Convey("prePrepare", func() {
			var res string
			pe = NewPackageExecutor("",
				func(ctx context.Context, pkg actions.Package) error {
					return pkg.Handler.Build(func() error { return nil })
				},
				func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
				func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return nil },
				func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
			)

			_, err := b.Build(ctx, pe, &Generator{Name: "first"})
			So(err, ShouldBeNil)
			So(res, ShouldEqual, "PrePost")
		})

		Convey("exec hooks", func() {
			Convey("ok", func() {
				var res string
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return nil },
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				So(err, ShouldBeNil)
				So(res, ShouldEqual, "PreExecPost")
			})

			Convey("err - pre", func() {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) {
						res += "Pre"
						return ctx, e
					},
					func(ctx context.Context, pkg actions.Package, execErr error) error {
						So(errors.Is(execErr, e), ShouldBeTrue)
						res += "Post"
						return nil
					},
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				So(errors.Is(err, e), ShouldBeTrue)
				So(res, ShouldEqual, "PrePost")
			})

			Convey("err - exec", func() {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					func(ctx context.Context, pkg actions.Package, execErr error) error {
						So(errors.Is(execErr, e), ShouldBeTrue)
						res += "Post"
						return nil
					},
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return e },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				So(errors.Is(err, e), ShouldBeTrue)
				So(res, ShouldEqual, "PreExecPost")
			})

			Convey("err - post", func() {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return e },
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				So(errors.Is(err, e), ShouldBeTrue)
				So(res, ShouldEqual, "PreExecPost")
			})
		})

		Convey("dependency not available", func() {
			pe = NewPackageExecutor("", nil, nil, nil,
				func(ctx context.Context, cfg *ExecutionConfig, drv *core.Derivation) error {
					if drv.Name == "second" {
						return fmt.Errorf("failed")
					}
					return nil
				},
			)

			pkgs, err := b.GeneratePackages(ctx, []generators.Generator{&Generator{
				Name: "first",
				Dependencies: []generators.Dependency{
					{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
				},
			}})
			So(err, ShouldBeNil)

			err = b.BuildPackages(ctx, pe, pkgs, true)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "dependency not available: second-")
		})
	})
}
