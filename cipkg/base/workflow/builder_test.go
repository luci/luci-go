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
		pe := NewPackageExecutor("", PackageExecutorConfig{
			ExecFn: func(context.Context, *ExecutionConfig, *core.Derivation) error { return nil },
		})

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

		t.Run("runtime dependency on already referenced derivation", func(t *ftt.Test) {
			dep, err := (&Generator{Name: "runtime_dep"}).Generate(ctx, generators.Platforms{})
			assert.Loosely(t, err, should.BeNil)

			pkgs, err := b.GeneratePackages(ctx, []generators.Generator{
				&Generator{Name: "shared"},
				&Generator{
					Name: "shared",
					Metadata: &core.Action_Metadata{
						RuntimeDeps: []*core.Action{dep},
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pkgs[0].DerivationID, should.Equal(pkgs[1].DerivationID))

			err = b.BuildPackages(ctx, pe, pkgs, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, func() { MustIncRefRecursiveRuntime(pkgs[1]) }, should.NotPanic)
			assert.Loosely(t, func() { MustDecRefRecursiveRuntime(pkgs[1]) }, should.NotPanic)
		})

		t.Run("distinct actions sharing derivation execute hooks and upload once per action", func(t *ftt.Test) {
			var built int
			var pre, post, uploaded []string
			pe = NewPackageExecutor("", PackageExecutorConfig{
				Storage: &testStorage{
					fetch: func(ctx context.Context, pkg actions.Package, id string) (string, error) {
						return "", core.ErrPackageNotExist
					},
					upload: func(ctx context.Context, pkg actions.Package, ids []string) (string, error) {
						uploaded = append(uploaded, pkg.Action.GetMetadata().GetCipd().GetName())
						return "hash", nil
					},
				},
				PreExecFn: func(ctx context.Context, pkg actions.Package) (context.Context, error) {
					pre = append(pre, pkg.Action.GetMetadata().GetCipd().GetName())
					return ctx, nil
				},
				PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error {
					post = append(post, pkg.Action.GetMetadata().GetCipd().GetName())
					return nil
				},
				ExecFn: func(context.Context, *ExecutionConfig, *core.Derivation) error {
					built++
					return nil
				},
			})

			pkgs, err := b.GeneratePackages(ctx, []generators.Generator{
				&Generator{
					Name:     "shared",
					Metadata: &core.Action_Metadata{Cipd: &core.Action_Metadata_CIPD{Name: "pkg/a"}},
				},
				&Generator{
					Name:     "shared",
					Metadata: &core.Action_Metadata{Cipd: &core.Action_Metadata_CIPD{Name: "pkg/b"}},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pkgs[0].DerivationID, should.Equal(pkgs[1].DerivationID))
			assert.Loosely(t, pkgs[0].ActionID, should.NotEqual(pkgs[1].ActionID))

			err = b.BuildPackages(ctx, pe, pkgs, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, built, should.Equal(1))
			assert.Loosely(t, pre, should.Match([]string{"pkg/a", "pkg/b"}))
			assert.Loosely(t, post, should.Match([]string{"pkg/a", "pkg/b"}))
			assert.Loosely(t, uploaded, should.Match([]string{"pkg/a", "pkg/b"}))

			removed, err := pkgs[0].Handler.TryRemove()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, removed, should.BeTrue)
		})

		t.Run("runtime dependency sharing derivation does not leak lock or skip upload", func(t *ftt.Test) {
			var built int
			var uploaded []string
			pe = NewPackageExecutor("", PackageExecutorConfig{
				Storage: &testStorage{
					fetch: func(ctx context.Context, pkg actions.Package, id string) (string, error) {
						if pkg.Action.GetMetadata().GetCipd().GetName() == "pkg/fetch" {
							return "hash-fetch", nil
						}
						return "", core.ErrPackageNotExist
					},
					upload: func(ctx context.Context, pkg actions.Package, ids []string) (string, error) {
						uploaded = append(uploaded, pkg.Action.GetMetadata().GetCipd().GetName())
						return "hash", nil
					},
				},
				ExecFn: func(context.Context, *ExecutionConfig, *core.Derivation) error {
					built++
					return nil
				},
			})

			fetchAct, err := (&Generator{
				Name:     "shared",
				Metadata: &core.Action_Metadata{Cipd: &core.Action_Metadata_CIPD{Name: "pkg/fetch"}},
			}).Generate(ctx, generators.Platforms{})
			assert.Loosely(t, err, should.BeNil)

			pkgs, err := b.GeneratePackages(ctx, []generators.Generator{
				&Generator{
					Name: "shared",
					Metadata: &core.Action_Metadata{
						Cipd:        &core.Action_Metadata_CIPD{Name: "pkg/out"},
						RuntimeDeps: []*core.Action{fetchAct},
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pkgs[0].DerivationID, should.Equal(pkgs[0].RuntimeDependencies[0].DerivationID))

			err = b.BuildPackages(ctx, pe, pkgs, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, built, should.Equal(0))
			assert.Loosely(t, uploaded, should.Match([]string{"pkg/fetch", "pkg/out"}))

			removed, err := pkgs[0].Handler.TryRemove()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, removed, should.BeTrue)
		})

		t.Run("prePrepare", func(t *ftt.Test) {
			var res string
			pe = NewPackageExecutor("", PackageExecutorConfig{
				PreExpandFn: func(ctx context.Context, pkg actions.Package) error {
					return pkg.Handler.Build(func() error { return nil })
				},
				PreExecFn:  func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
				PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return nil },
				ExecFn:     func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
			})

			_, err := b.Build(ctx, pe, &Generator{Name: "first"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Equal("PrePost"))
		})

		t.Run("storage", func(t *ftt.Test) {
			t.Run("fetch hit skips build", func(t *ftt.Test) {
				var res string
				pe = NewPackageExecutor("", PackageExecutorConfig{
					Storage: &testStorage{
						fetch: func(ctx context.Context, pkg actions.Package, id string) (string, error) {
							res += "Fetch"
							return "hash-1", nil
						},
						upload: func(ctx context.Context, pkg actions.Package, ids []string) (string, error) {
							res += "Upload"
							return "hash-1", nil
						},
					},
					PreExecFn:  func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return nil },
					ExecFn:     func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				})

				_, err := b.Build(ctx, pe, &Generator{Name: "cached"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("FetchPrePostUpload"))
			})

			t.Run("fetch miss builds and uploads", func(t *ftt.Test) {
				var res string
				pe = NewPackageExecutor("", PackageExecutorConfig{
					Storage: &testStorage{
						fetch: func(ctx context.Context, pkg actions.Package, id string) (string, error) {
							res += "Fetch"
							return "", core.ErrPackageNotExist
						},
						upload: func(ctx context.Context, pkg actions.Package, ids []string) (string, error) {
							res += "Upload"
							return "hash-2", nil
						},
					},
					PreExecFn:  func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return nil },
					ExecFn:     func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				})

				_, err := b.Build(ctx, pe, &Generator{Name: "uncached"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("FetchPreFetchExecPostUpload"))
			})

			t.Run("stable storage skips dependent build when dependency content hash matches", func(t *ftt.Test) {
				remote := make(map[string]string)

				makeGraph := func(depVersion string) generators.Generator {
					dep := &Generator{
						Name: "dep",
						Args: []string{"build-dep", depVersion},
					}
					mid := &Generator{
						Name: "mid",
						Args: []string{"wrap", actions.DepRef("dep")},
						Dependencies: []generators.Dependency{
							{Generator: dep, Type: generators.DepsBuildHost},
						},
					}
					return &Generator{
						Name: "root",
						Args: []string{"build-root", actions.DepRef("mid")},
						Dependencies: []generators.Dependency{
							{Generator: mid, Type: generators.DepsBuildHost},
						},
					}
				}

				newBackend := func() RemoteStorage {
					return &testStorage{
						fetch: func(_ context.Context, _ actions.Package, id string) (string, error) {
							if h, ok := remote[id]; ok {
								return h, nil
							}
							return "", core.ErrPackageNotExist
						},
						upload: func(_ context.Context, pkg actions.Package, ids []string) (string, error) {
							switch pkg.Action.Name {
							case "dep":
								// Do not store dep in remote map so dep must rebuild when its
								// action changes, returning the same content hash.
								return "dep-content-hash", nil
							case "mid":
								// Unstored intermediate package.
								return "", nil
							default:
								for _, id := range ids {
									remote[id] = "root-content-hash"
								}
								return "root-content-hash", nil
							}
						},
					}
				}

				// Run 1: builds dep, mid, and root, and uploads root with both
				// content-addressed and relocatable IDs.
				ap1 := actions.NewActionProcessor()
				pm1, err := NewLocalPackageManager(t.TempDir())
				assert.Loosely(t, err, should.BeNil)
				b1 := NewBuilder(generators.Platforms{}, pm1, ap1)
				var built1 []string
				pe1 := NewPackageExecutor(t.TempDir(), PackageExecutorConfig{
					Storage: NewStableStorage(ap1, newBackend()),
					ExecFn: func(_ context.Context, _ *ExecutionConfig, drv *core.Derivation) error {
						built1 = append(built1, drv.Name)
						return nil
					},
				})
				_, err = b1.Build(ctx, pe1, makeGraph("v1"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, built1, should.Match([]string{"dep", "mid", "root"}))

				// Run 2: dep action changes ("v2") but produces the same content hash;
				// dep and mid build, while root hits remote storage before execution
				// and attaches the new relocatable ID during Upload.
				ap2 := actions.NewActionProcessor()
				pm2, err := NewLocalPackageManager(t.TempDir())
				assert.Loosely(t, err, should.BeNil)
				b2 := NewBuilder(generators.Platforms{}, pm2, ap2)
				var built2 []string
				pe2 := NewPackageExecutor(t.TempDir(), PackageExecutorConfig{
					Storage: NewStableStorage(ap2, newBackend()),
					ExecFn: func(_ context.Context, _ *ExecutionConfig, drv *core.Derivation) error {
						built2 = append(built2, drv.Name)
						return nil
					},
				})
				_, err = b2.Build(ctx, pe2, makeGraph("v2"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, built2, should.Match([]string{"dep", "mid"}))

				// Run 3: repeating "v2" in a fresh storage dir hits root's newly
				// attached relocatable ID directly during Expand without building any
				// dependencies.
				ap3 := actions.NewActionProcessor()
				pm3, err := NewLocalPackageManager(t.TempDir())
				assert.Loosely(t, err, should.BeNil)
				b3 := NewBuilder(generators.Platforms{}, pm3, ap3)
				var built3 []string
				pe3 := NewPackageExecutor(t.TempDir(), PackageExecutorConfig{
					Storage: NewStableStorage(ap3, newBackend()),
					ExecFn: func(_ context.Context, _ *ExecutionConfig, drv *core.Derivation) error {
						built3 = append(built3, drv.Name)
						return nil
					},
				})
				_, err = b3.Build(ctx, pe3, makeGraph("v2"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, built3, should.BeEmpty)
			})
		})

		t.Run("preExpand deduplicated across shared dependencies", func(t *ftt.Test) {
			var expanded []string
			pe = NewPackageExecutor("", PackageExecutorConfig{
				PreExpandFn: func(ctx context.Context, pkg actions.Package) error {
					expanded = append(expanded, pkg.Derivation.Name)
					return nil
				},
				ExecFn: func(context.Context, *ExecutionConfig, *core.Derivation) error { return nil },
			})

			shared := &Generator{Name: "shared"}
			_, err := b.Build(ctx, pe, &Generator{
				Name: "root",
				Dependencies: []generators.Dependency{
					{Generator: &Generator{Name: "left", Dependencies: []generators.Dependency{{Generator: shared, Type: generators.DepsBuildHost}}}, Type: generators.DepsBuildHost},
					{Generator: &Generator{Name: "right", Dependencies: []generators.Dependency{{Generator: shared, Type: generators.DepsBuildHost}}}, Type: generators.DepsBuildHost},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, expanded, should.Match([]string{"root", "left", "shared", "right"}))
		})

		t.Run("exec hooks", func(t *ftt.Test) {
			t.Run("ok", func(t *ftt.Test) {
				var res string
				pe = NewPackageExecutor("", PackageExecutorConfig{
					PreExecFn:  func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return nil },
					ExecFn:     func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				})

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res, should.Equal("PreExecPost"))
			})

			t.Run("err - pre", func(t *ftt.Test) {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("", PackageExecutorConfig{
					PreExecFn: func(ctx context.Context, pkg actions.Package) (context.Context, error) {
						res += "Pre"
						return ctx, e
					},
					PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error {
						assert.Loosely(t, errors.Is(execErr, e), should.BeTrue)
						res += "Post"
						return nil
					},
					ExecFn: func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				})

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, errors.Is(err, e), should.BeTrue)
				assert.Loosely(t, res, should.Equal("PrePost"))
			})

			t.Run("err - exec", func(t *ftt.Test) {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("", PackageExecutorConfig{
					PreExecFn: func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error {
						assert.Loosely(t, errors.Is(execErr, e), should.BeTrue)
						res += "Post"
						return nil
					},
					ExecFn: func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return e },
				})

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, errors.Is(err, e), should.BeTrue)
				assert.Loosely(t, res, should.Equal("PreExecPost"))
			})

			t.Run("err - post", func(t *ftt.Test) {
				var res string
				e := errors.New("err")
				pe = NewPackageExecutor("", PackageExecutorConfig{
					PreExecFn:  func(ctx context.Context, pkg actions.Package) (context.Context, error) { res += "Pre"; return ctx, nil },
					PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error { res += "Post"; return e },
					ExecFn:     func(context.Context, *ExecutionConfig, *core.Derivation) error { res += "Exec"; return nil },
				})

				_, err := b.Build(ctx, pe, &Generator{Name: "first"})
				assert.Loosely(t, errors.Is(err, e), should.BeTrue)
				assert.Loosely(t, res, should.Equal("PreExecPost"))
			})
		})

		t.Run("dependency not available", func(t *ftt.Test) {
			pe = NewPackageExecutor("", PackageExecutorConfig{
				ExecFn: func(ctx context.Context, cfg *ExecutionConfig, drv *core.Derivation) error {
					if drv.Name == "second" {
						return fmt.Errorf("failed")
					}
					return nil
				},
			})

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

type testStorage struct {
	resolve func(ctx context.Context, pkg actions.Package, id string) (string, error)
	fetch   func(ctx context.Context, pkg actions.Package, id string) (string, error)
	upload  func(ctx context.Context, pkg actions.Package, ids []string) (string, error)
}

func (s *testStorage) Resolve(ctx context.Context, pkg actions.Package, id string) (string, error) {
	if s.resolve != nil {
		return s.resolve(ctx, pkg, id)
	}
	return "", core.ErrPackageNotExist
}

func (s *testStorage) Fetch(ctx context.Context, pkg actions.Package, id string) (string, error) {
	return s.fetch(ctx, pkg, id)
}

func (s *testStorage) Upload(ctx context.Context, pkg actions.Package, ids []string) (string, error) {
	return s.upload(ctx, pkg, ids)
}
