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

	"go.chromium.org/luci/common/exec/execmock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
)

func TestExecutor(t *testing.T) {
	ftt.Run("Test executor", t, func(t *ftt.Test) {
		ctx := execmock.Init(context.Background())
		uses := execmock.Simple.Mock(ctx, execmock.SimpleInput{})
		out := t.TempDir()

		t.Run("ok", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			usage := uses.Snapshot()
			assert.Loosely(t, usage[0].Args, should.Match([]string{"bin", "-args", "1"}))
			assert.Loosely(t, usage[0].Env.Sorted(), should.Match([]string{"env1=1", "env2=2", fmt.Sprintf("out=%s", out)}))
		})

		t.Run("env - override out", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			usage := uses.Snapshot()
			assert.Loosely(t, usage[0].Args, should.Match([]string{"bin", "-args", "1"}))
			assert.Loosely(t, usage[0].Env.Sorted(), should.Match([]string{"env1=1", "env2=2", fmt.Sprintf("out=%s", out)}))
		})
	})
}

func TestBuilder(t *testing.T) {
	ftt.Run("Test Builder", t, func(t *ftt.Test) {
		ctx := context.Background()

		pm, err := NewLocalPackageManager(t.TempDir())
		assert.Loosely(t, err, should.BeNil)

		b := NewBuilder(generators.Platforms{}, pm, actions.NewActionProcessor())
		pe := NewPackageExecutor("", nil, nil, nil, func(context.Context, *ExecutionConfig, *core.Derivation) error { return nil })

		t.Run("ok", func(t *ftt.Test) {
			pkg, err := b.Build(ctx, pe, &Generator{
				Name: "first",
				Dependencies: []generators.Dependency{
					{Generator: &Generator{Name: "second"}, Type: generators.DepsBuildHost, Runtime: true},
				},
			})
			assert.Loosely(t, err, should.BeNil)

			// all packages are available
			assert.Loosely(t, func() { MustIncRefRecursiveRuntime(pkg) }, should.NotPanic)
			assert.Loosely(t, func() { MustDecRefRecursiveRuntime(pkg) }, should.NotPanic)
			assert.Loosely(t, pkg.Action.Name, should.Equal("first"))
		})

		t.Run("prePrepare", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("PrePost"))
		})

		t.Run("exec hooks", func(t *ftt.Test) {
			t.Run("ok", func(t *ftt.Test) {
				var res string
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return nil },
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("PreExecPost"))
			})

			t.Run("err - pre", func(t *ftt.Test) {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) {
						res += "Pre"
						return ctx, e
					},
					func(ctx context.Context, pkg actions.Package, execErr error) error {
						assert.Loosely(t, errors.Is(execErr, e), should.BeTrue)
						res += "Post"
						return nil
					},
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, errors.Is(err, e), should.BeTrue)
				assert.Loosely(t, res, should.Equal("PrePost"))
			})

			t.Run("err - exec", func(t *ftt.Test) {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					func(ctx context.Context, pkg actions.Package, execErr error) error {
						assert.Loosely(t, errors.Is(execErr, e), should.BeTrue)
						res += "Post"
						return nil
					},
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return e },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, errors.Is(err, e), should.BeTrue)
				assert.Loosely(t, res, should.Equal("PreExecPost"))
			})

			t.Run("err - post", func(t *ftt.Test) {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("",
					nil,
					func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return e },
					func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				)

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, errors.Is(err, e), should.BeTrue)
				assert.Loosely(t, res, should.Equal("PreExecPost"))
			})
		})

		t.Run("dependency not available", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			err = b.BuildPackages(ctx, pe, pkgs, true)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("dependency not available: second-"))
		})
	})
}
