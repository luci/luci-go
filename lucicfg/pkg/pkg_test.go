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

package pkg

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/lucicfg/fileset"
	"go.chromium.org/luci/lucicfg/lockfilepb"
)

func TestEntryOnDisk(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	repoMgr := &TestRepoManager{
		Root: prepDisk(t, map[string]string{
			"remote/v1/a/PACKAGE.star": `
				pkg.declare(name = "@remote/a", lucicfg = "1.2.5")
				pkg.resources(["**/*.cfg"])
				pkg.depend(
					name = "@remote/b",
					source = pkg.source.local(
						path = "../b",
					)
				)
			`,
			"remote/v1/a/test.cfg": "",
			"remote/v1/b/PACKAGE.star": `
				pkg.declare(name = "@remote/b", lucicfg = "1.2.6")
			`,
		}),
	}

	t.Run("Legacy mode", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config":     `# Denotes repo root`,
			"a/b/c/main.star": `print("Hi")`,
		})

		entry, lockfile, err := EntryOnDisk(ctx, filepath.Join(tmp, "a/b/c/main.star"), repoMgr, nil)
		assert.NoErr(t, err)
		assert.That(t, entry.Local.DiskPath, should.Equal(filepath.Join(tmp, "a/b/c")))

		_, src, err := entry.Main(ctx, "main.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal(`print("Hi")`))

		assert.Loosely(t, entry.Deps, should.HaveLength(0))
		assert.That(t, entry.Path, should.Equal("a/b/c"))
		assert.That(t, entry.Script, should.Equal("main.star"))
		assert.Loosely(t, entry.LucicfgVersionConstraints, should.HaveLength(0))

		sources, err := entry.Local.Sources()
		assert.NoErr(t, err)
		assert.That(t, sources, should.Match([]string{
			"main.star",
		}))

		assert.That(t, lockfile, should.Match(&lockfilepb.Lockfile{
			Packages: []*lockfilepb.Lockfile_Package{
				{
					Name:        "@__main__",
					Resources:   []string{"**/*"},
					Entrypoints: []string{"main.star"},
				},
			},
		}))
	})

	t.Run("Loads PACKAGE.star", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config": `# Denotes repo root`,
			"a/b/PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
				pkg.entrypoint("c/main.star")
			`,
			"a/b/c/main.star": `print("Hi")`,
		})

		entry, lockfile, err := EntryOnDisk(ctx, filepath.Join(tmp, "a/b/c/main.star"), repoMgr, nil)
		assert.NoErr(t, err)
		assert.That(t, entry.Local.DiskPath, should.Equal(filepath.Join(tmp, "a/b")))

		_, src, err := entry.Main(ctx, "c/main.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal(`print("Hi")`))

		assert.Loosely(t, entry.Deps, should.HaveLength(0))
		assert.That(t, entry.Path, should.Equal("a/b"))
		assert.That(t, entry.Script, should.Equal("c/main.star"))
		assert.That(t, entry.LucicfgVersionConstraints, should.Match([]LucicfgVersionConstraint{
			{
				Min:     LucicfgVersion{1, 2, 3},
				Package: "@some/pkg",
				Main:    true,
			},
		}))

		sources, err := entry.Local.Sources()
		assert.NoErr(t, err)
		assert.That(t, sources, should.Match([]string{
			"PACKAGE.star",
			"c/main.star",
		}))

		assert.That(t, lockfile, should.Match(&lockfilepb.Lockfile{
			Packages: []*lockfilepb.Lockfile_Package{
				{
					Name:        "@some/pkg",
					Lucicfg:     "1.2.3",
					Entrypoints: []string{"c/main.star"},
				},
			},
		}))
	})

	t.Run("Loads dependencies", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config": `# Denotes repo root`,
			"a/b/PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
				pkg.entrypoint("c/main.star")
				pkg.depend(
					name = "@local",
					source = pkg.source.local(
						path = "../dep",
					)
				)
			`,
			"a/b/c/main.star": `print("Hi")`,

			"a/dep/PACKAGE.star": `
				pkg.declare(name = "@local", lucicfg = "1.2.4")
				pkg.depend(
					name = "@remote/a",
					source = pkg.source.googlesource(
						host = "ignored-in-test",
						repo = "remote",
						ref = "ignored-in-test",
						path = "a",
						revision = "v1",
					)
				)
			`,
		})

		entry, lockfile, err := EntryOnDisk(ctx, filepath.Join(tmp, "a/b/c/main.star"), repoMgr, nil)
		assert.NoErr(t, err)

		assert.That(t, slices.Sorted(maps.Keys(entry.Deps)), should.Match([]string{
			"local",
			"remote/a",
			"remote/b",
		}))

		assert.That(t, entry.LucicfgVersionConstraints, should.Match([]LucicfgVersionConstraint{
			{Min: LucicfgVersion{1, 2, 3}, Package: "@some/pkg", Main: true},
			{Min: LucicfgVersion{1, 2, 4}, Package: "@local"},
			{Min: LucicfgVersion{1, 2, 5}, Package: "@remote/a"},
			{Min: LucicfgVersion{1, 2, 6}, Package: "@remote/b"},
		}))

		assert.That(t, entry.DepGraph, should.Match(map[string][]string{
			"@local":    {"@remote/a"},
			"@remote/a": {"@remote/b"},
			"@remote/b": nil,
			"@some/pkg": {"@local"},
		}))

		// Doesn't report the nested package as part of the main package.
		sources, err := entry.Local.Sources()
		assert.NoErr(t, err)
		assert.That(t, sources, should.Match([]string{
			"PACKAGE.star",
			"c/main.star",
		}))

		assert.That(t, lockfile, should.Match(&lockfilepb.Lockfile{
			Packages: []*lockfilepb.Lockfile_Package{
				{
					Name:        "@some/pkg",
					Lucicfg:     "1.2.3",
					Deps:        []string{"@local"},
					Entrypoints: []string{"c/main.star"},
				},
				{
					Name: "@local",
					Source: &lockfilepb.Lockfile_Package_Source{
						Path: "../dep",
					},
					Lucicfg: "1.2.4",
					Deps:    []string{"@remote/a"},
				},
				{
					Name: "@remote/a",
					Source: &lockfilepb.Lockfile_Package_Source{
						Repo:     "https://ignored-in-test.googlesource.com/remote/+/ignored-in-test",
						Revision: "v1",
						Path:     "a",
					},
					Lucicfg:   "1.2.5",
					Deps:      []string{"@remote/b"},
					Resources: []string{"**/*.cfg"},
				},
				{
					Name: "@remote/b",
					Source: &lockfilepb.Lockfile_Package_Source{
						Repo:     "https://ignored-in-test.googlesource.com/remote/+/ignored-in-test",
						Revision: "v1",
						Path:     "b",
					},
					Lucicfg: "1.2.6",
				},
			},
		}))
	})

	t.Run("Borked PACKAGE.star", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config":  `# Denotes repo root`,
			"PACKAGE.star": ``,
		})
		_, _, err := EntryOnDisk(ctx, filepath.Join(tmp, "main.star"), repoMgr, nil)
		assert.That(t, err, should.ErrLike(`PACKAGE.star must call pkg.declare(...)`))
	})

	t.Run("Missing entry point", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config": `# Denotes repo root`,
			"PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
				pkg.entrypoint("missing.star")
			`,
		})
		_, _, err := EntryOnDisk(ctx, filepath.Join(tmp, "missing.star"), repoMgr, nil)
		assert.That(t, err, should.ErrLike(`entry point "missing.star": no such file in the package`))
	})

	t.Run("Undeclared entry point", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config": `# Denotes repo root`,
			"PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
				pkg.entrypoint("another1.star")
				pkg.entrypoint("another2.star")
			`,
			"main.star":     `print("Hi")`,
			"another1.star": `print("Hi")`,
			"another2.star": `print("Hi")`,
		})
		_, _, err := EntryOnDisk(ctx, filepath.Join(tmp, "main.star"), repoMgr, nil)
		assert.That(t, err, should.ErrLike(
			`main.star is not declared as a pkg.entrypoint(...) in PACKAGE.star and thus cannot be executed. Available entrypoints: [another1.star another2.star]`))
	})

	t.Run("Local dependency outside of the repo root", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			"a/.git/config": `# Denotes repo root`,
			"a/pkg/PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
				pkg.depend(
					name = "@local",
					source = pkg.source.local(
						path = "../../b",
					)
				)
				pkg.entrypoint("main.star")
			`,
			"a/pkg/main.star": `print("Hi")`,
			"b/PACKAGE.star": `
				pkg.declare(name = "@local", lucicfg = "1.2.3")
			`,
		})

		_, _, err := EntryOnDisk(ctx, filepath.Join(tmp, "a/pkg/main.star"), nil, nil)
		assert.That(t, err, should.ErrLike(
			`bad dependency on "@local": a local dependency must not point outside of the repository it is declared in`))
	})

	t.Run("Local dependency inside a git submodule", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config": `# Denotes repo root`,
			"pkg/PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
				pkg.depend(
					name = "@local",
					source = pkg.source.local(
						path = "../submod",
					)
				)
				pkg.entrypoint("main.star")
			`,
			"pkg/main.star":      `print("Hi")`,
			"submod/.git/config": `# Denotes a submodule root`,
			"submod/PACKAGE.star": `
				pkg.declare(name = "@local", lucicfg = "1.2.3")
			`,
		})

		_, _, err := EntryOnDisk(ctx, filepath.Join(tmp, "pkg/main.star"), nil, nil)
		assert.That(t, err, should.ErrLike(
			`bad dependency on "@local": a local dependency should not reside in a git submodule`))
	})

	t.Run("Transitive local dependency inside a git submodule", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config": `# Denotes repo root`,
			"pkg/PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
				pkg.depend(
					name = "@local-1",
					source = pkg.source.local(
						path = "../dep",
					)
				)
				pkg.entrypoint("main.star")
			`,
			"pkg/main.star": `print("Hi")`,
			"dep/PACKAGE.star": `
				pkg.declare(name = "@local-1", lucicfg = "1.2.3")
				pkg.depend(
					name = "@local-2",
					source = pkg.source.local(
						path = "../submod",
					)
				)
			`,
			"submod/.git/config": `# Denotes a submodule root`,
			"submod/PACKAGE.star": `
				pkg.declare(name = "@local-2", lucicfg = "1.2.3")
			`,
		})

		_, _, err := EntryOnDisk(ctx, filepath.Join(tmp, "pkg/main.star"), nil, nil)
		assert.That(t, err, should.ErrLike(
			`bad dependency on "@local-2": a local dependency should not reside in a git submodule`))
	})
}

func TestPackageOnDisk(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("Legacy mode", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			"a/b/c/main.star": `print("Hi")`,
		})

		pkg, err := PackageOnDisk(ctx, filepath.Join(tmp, "a/b/c"))
		assert.NoErr(t, err)
		assert.That(t, pkg.DiskPath, should.Equal(filepath.Join(tmp, "a/b/c")))

		_, src, err := pkg.Code(ctx, "main.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal(`print("Hi")`))

		assert.That(t, pkg.Definition, should.Match(&Definition{
			Name:      LegacyPackageNamePlaceholder,
			Resources: []string{"**/*"},
		}))
	})

	t.Run("Loads PACKAGE.star", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			"a/b/PACKAGE.star": `
				pkg.declare(name = "@some/pkg", lucicfg = "1.2.3")
			`,
			"a/b/c/main.star": `print("Hi")`,
		})

		pkg, err := PackageOnDisk(ctx, filepath.Join(tmp, "a/b"))
		assert.NoErr(t, err)
		assert.That(t, pkg.DiskPath, should.Equal(filepath.Join(tmp, "a/b")))

		_, src, err := pkg.Code(ctx, "c/main.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal(`print("Hi")`))

		assert.That(t, pkg.Definition, should.Match(&Definition{
			Name:              "@some/pkg",
			MinLucicfgVersion: LucicfgVersion{1, 2, 3},
			ResourcesSet:      &fileset.Set{},
		}))
	})

	t.Run("Borked PACKAGE.star", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			"PACKAGE.star": ``,
		})
		_, err := PackageOnDisk(ctx, tmp)
		assert.That(t, err, should.ErrLike(`PACKAGE.star must call pkg.declare(...)`))
	})
}

func TestRepoOverrideFromSpec(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spec string
		out  string
		err  string
	}{
		{"https://host.example.com/repo/+/refs/heads/main", "host:repo:refs/heads/main", ""},
		{"host.example.com/repo/+/refs/heads/main", "host:repo:refs/heads/main", ""},
		{"host/repo/+/refs/heads/main", "host:repo:refs/heads/main", ""},

		{"", "", "doesn't look like a repository URL"},
		{"host", "", "doesn't look like a repository URL"},
		{"host/", "", "doesn't look like a repository URL"},
		{"host/+/refs/heads/main", "", "doesn't look like a repository URL"},
		{"host/repo", "", "the repo spec is missing a ref"},
	}

	for _, cs := range cases {
		t.Run(cs.spec, func(t *testing.T) {
			out, err := RepoOverrideFromSpec(cs.spec, ".")
			if cs.err != "" {
				assert.That(t, err, should.ErrLike(cs.err))
			} else {
				assert.NoErr(t, err)
				assert.That(t, fmt.Sprintf("%s:%s:%s", out.Host, out.Repo, out.Ref), should.Equal(cs.out))
			}
		})
	}
}

func prepDisk(t *testing.T, files map[string]string) string {
	tmp := t.TempDir()
	for path, body := range files {
		abs := filepath.Join(tmp, filepath.FromSlash(path))
		assert.NoErr(t, os.MkdirAll(filepath.Dir(abs), 0750))
		assert.NoErr(t, os.WriteFile(abs, []byte(deindent(body)), 0660))
	}
	return tmp
}
