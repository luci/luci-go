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
	"path"
	"slices"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/starlark/interpreter"
)

func TestDiscoverDeps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("Local only: OK", func(t *testing.T) {
		repos, local := prepRepos(t,
			map[string]string{
				"main/PACKAGE.star": `Not actually loaded, was already loaded`,
				"local/PACKAGE.star": `
					pkg.declare(
						name = "@local-1",
						lucicfg = "1.2.4",
					)
					pkg.depend(
						name = "@local-2",
						source = pkg.source.local(
							path = "deeper",
						)
					)
					pkg.depend(
						name = "@local-3",
						source = pkg.source.local(
							path = "../outside",
						)
					)
					pkg.resources(["**/*.cfg"])
				`,
				"local/file.star":  `Works`,
				"local/file.cfg":   `Also works`,
				"local/hidden.bin": `Doesn't work`,
				"local/deeper/PACKAGE.star": `
					pkg.declare(
						name = "@local-2",
						lucicfg = "1.2.5",
					)
					pkg.depend(
						name = "@local-1",  # cycles are fine
						source = pkg.source.local(
							path = "..",
						)
					)
					pkg.depend(
						name = "@local-3",  # "diamond" deps are fine
						source = pkg.source.local(
							path = "../../outside",
						)
					)
				`,
				"local/deeper/file.star": `Works`,
				"outside/PACKAGE.star": `
					pkg.declare(
						name = "@local-3",
						lucicfg = "1.2.6",
					)
				`,
			},
			nil,
		)

		deps, err := discoverDeps(ctx, &DepContext{
			Package:     "@main",
			Version:     PinnedVersion,
			Repo:        local,
			Path:        "main",
			RepoManager: repos,
			Known: &Definition{
				Name:              "@main",
				MinLucicfgVersion: [3]int{1, 2, 3},
				Deps: []*DepDecl{
					{
						Name:      "@local-1",
						LocalPath: "../local",
					},
				},
			},
		})

		assert.NoErr(t, err)
		assert.That(t, deps, should.Match([]*Dep{
			{
				Package: "@local-1",
				Min:     [3]int{1, 2, 4},
				Code:    deps[0].Code,
			},
			{
				Package: "@local-2",
				Min:     [3]int{1, 2, 5},
				Code:    deps[1].Code,
			},
			{
				Package: "@local-3",
				Min:     [3]int{1, 2, 6},
				Code:    deps[2].Code,
			},
		}))

		// Verify can read Starlark files and resources.
		local1 := deps[0].Code
		_, src, err := local1(ctx, "file.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal("Works"))
		_, src, err = local1(ctx, "file.cfg")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal("Also works"))

		// Can't read undeclared resource.
		_, _, err = local1(ctx, "hidden.bin")
		assert.That(t, err, should.ErrLike(
			`this non-starlark file is not declared as a resource in pkg.resources(...) in PACKAGE.star of "@local-1" and cannot be loaded`))

		// Can't read missing files.
		_, _, err = local1(ctx, "unknown.star")
		assert.That(t, err, should.Equal(interpreter.ErrNoModule))

		// CAN directly read files from the nested packages. Strictly speaking this
		// is wrong, but preventing it may be non-trivial, so we allow it for now.
		_, src, err = local1(ctx, "deeper/file.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal("Works"))
	})

	t.Run("Local only: missing dep", func(t *testing.T) {
		repos, local := prepRepos(t,
			map[string]string{
				"main/PACKAGE.star": `Not actually loaded, was already loaded`,
				"local/PACKAGE.star": `
					pkg.declare(
						name = "@local-1",
						lucicfg = "1.2.4",
					)
					pkg.depend(
						name = "@local-2",
						source = pkg.source.local(
							path = "missing",
						)
					)
				`,
			},
			nil,
		)

		_, err := discoverDeps(ctx, &DepContext{
			Package:     "@main",
			Version:     PinnedVersion,
			Repo:        local,
			Path:        "main",
			RepoManager: repos,
			Known: &Definition{
				Name:              "@main",
				MinLucicfgVersion: [3]int{1, 2, 3},
				Deps: []*DepDecl{
					{
						Name:      "@local-1",
						LocalPath: "../local",
					},
				},
			},
		})
		// TODO: Add more context to the error.
		assert.That(t, err, should.ErrLike("local file: local/missing/PACKAGE.star: no such file"))
	})

	t.Run("Local only: loading wrong package", func(t *testing.T) {
		repos, local := prepRepos(t,
			map[string]string{
				"main/PACKAGE.star": `Not actually loaded, was already loaded`,
				"local/PACKAGE.star": `
					pkg.declare(
						name = "@something-else",
						lucicfg = "1.2.4",
					)
				`,
			},
			nil,
		)

		_, err := discoverDeps(ctx, &DepContext{
			Package:     "@main",
			Version:     PinnedVersion,
			Repo:        local,
			Path:        "main",
			RepoManager: repos,
			Known: &Definition{
				Name:              "@main",
				MinLucicfgVersion: [3]int{1, 2, 3},
				Deps: []*DepDecl{
					{
						Name:      "@local-1",
						LocalPath: "../local",
					},
				},
			},
		})
		// TODO: Add more context to the error.
		assert.That(t, err, should.ErrLike(`expected to find package "@local-1", but found "@something-else" instead`))
	})

	t.Run("Local only: jumping outside of repo", func(t *testing.T) {
		repos, local := prepRepos(t,
			map[string]string{
				"main/PACKAGE.star": `Not actually loaded, was already loaded`,
				"local/PACKAGE.star": `
					pkg.declare(
						name = "@local-1",
						lucicfg = "1.2.4",
					)
					pkg.depend(
						name = "@local-2",
						source = pkg.source.local(
							path = "../..",
						)
					)
				`,
			},
			nil,
		)

		_, err := discoverDeps(ctx, &DepContext{
			Package:     "@main",
			Version:     PinnedVersion,
			Repo:        local,
			Path:        "main",
			RepoManager: repos,
			Known: &Definition{
				Name:              "@main",
				MinLucicfgVersion: [3]int{1, 2, 3},
				Deps: []*DepDecl{
					{
						Name:      "@local-1",
						LocalPath: "../local",
					},
				},
			},
		})
		// TODO: Add more context to the error.
		assert.That(t, err, should.ErrLike(`local dependency on "@local-2" points to a path outside the repository: "../.."`))
	})

	t.Run("Remote: prefetch + loader OK", func(t *testing.T) {
		repos, local := prepRepos(t,
			nil,
			map[string]string{
				// At the root of the repo.
				"repo/v1/PACKAGE.star": `
					pkg.declare(
						name = "@remote-1",
						lucicfg = "1.2.4",
					)
					pkg.resources(["**/*.cfg"])
				`,
				"repo/v1/file.star":           `Works`,
				"repo/v1/file.cfg":            `Also works`,
				"repo/v1/deeper/another.star": "",
				"repo/v1/deeper/another.cfg":  "",
				"repo/v1/hidden.bin":          "",

				// Deeper inside the repo.
				"repo/v2/deep/PACKAGE.star": `
					pkg.declare(
						name = "@remote-2",
						lucicfg = "1.2.5",
					)
					pkg.resources(["**/*.cfg"])
				`,
				"repo/v2/deep/file.star":           "",
				"repo/v2/deep/file.cfg":            "",
				"repo/v2/deep/deeper/another.star": "",
				"repo/v2/deep/deeper/another.cfg":  "",
				"repo/v2/deep/hidden.bin":          "",
			},
		)

		deps, err := discoverDeps(ctx, &DepContext{
			Package:     "@main",
			Version:     PinnedVersion,
			Repo:        local,
			Path:        "main",
			RepoManager: repos,
			Known: &Definition{
				Name:              "@main",
				MinLucicfgVersion: [3]int{1, 2, 3},
				Deps: []*DepDecl{
					{
						Name:     "@remote-1",
						Host:     "ignored-in-test",
						Repo:     "repo",
						Ref:      "ignored-in-test",
						Path:     ".",
						Revision: "v1",
					},
					{
						Name:     "@remote-2",
						Host:     "ignored-in-test",
						Repo:     "repo",
						Ref:      "ignored-in-test",
						Path:     "deep",
						Revision: "v2",
					},
				},
			},
		})

		assert.NoErr(t, err)
		assert.That(t, deps, should.Match([]*Dep{
			{
				Package: "@remote-1",
				Min:     [3]int{1, 2, 4},
				Code:    deps[0].Code,
			},
			{
				Package: "@remote-2",
				Min:     [3]int{1, 2, 5},
				Code:    deps[1].Code,
			},
		}))

		// Verify can read Starlark files and resources.
		remote1 := deps[0].Code
		_, src, err := remote1(ctx, "file.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal("Works"))
		_, src, err = remote1(ctx, "file.cfg")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal("Also works"))

		// Can't read undeclared resource.
		_, _, err = remote1(ctx, "hidden.bin")
		assert.That(t, err, should.ErrLike(
			`this non-starlark file is not declared as a resource in pkg.resources(...) in PACKAGE.star of "@remote-1" and cannot be loaded`))

		// Can't read missing files.
		_, _, err = remote1(ctx, "unknown.star")
		assert.That(t, err, should.Equal(interpreter.ErrNoModule))

		// Prefetched correct files.
		repo, err := repos.Repo(ctx, RepoKey{
			Host: "ignored-in-test",
			Repo: "repo",
			Ref:  "ignored-in-test",
		})
		assert.NoErr(t, err)
		assert.That(t, repo.(*TestRepo).Prefetched(), should.Match([]string{
			"v1/PACKAGE.star",
			"v1/deeper/another.cfg",
			"v1/deeper/another.star",
			"v1/file.cfg",
			"v1/file.star",
			"v2/deep/PACKAGE.star",
			"v2/deep/deeper/another.cfg",
			"v2/deep/deeper/another.star",
			"v2/deep/file.cfg",
			"v2/deep/file.star",
		}))
	})
}

func TestVersionResolution(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mustBeOK := func(t *testing.T, spec string, expected []string) {
		deps, err := runSpec(t, ctx, spec)
		assert.NoErr(t, err)
		assert.That(t, probeVersions(ctx, deps), should.Match(expected))
	}

	t.Run("Just root", func(t *testing.T) {
		mustBeOK(t,
			`
			r:
			`,
			nil,
		)
	})

	t.Run("Local deps only", func(t *testing.T) {
		mustBeOK(t,
			`
			r: a b
			a@local: c
			b@local: c
			c@local: r
			`,
			[]string{
				"a0@local",
				"b0@local",
				"c0@local",
			},
		)
	})

	t.Run("Simple remote deps", func(t *testing.T) {
		mustBeOK(t,
			`
			r: a1@remote1 b1@remote2
			a1@remote1: c1@remote3
			b1@remote2: c1@remote3
			c1@remote3:
			`,
			[]string{
				"a1@remote1",
				"b1@remote2",
				"c1@remote3",
			},
		)
	})

	t.Run("Picks most recent version", func(t *testing.T) {
		mustBeOK(t,
			`
			r: a1@remote1 b1@remote2
			a1@remote1: c1@remote3
			b1@remote2: c2@remote3  # newer
			c1@remote3:
			c2@remote3:
			`,
			[]string{
				"a1@remote1",
				"b1@remote2",
				"c2@remote3", // picked newer
			},
		)
	})

	t.Run("Respects version of local deps of remote deps", func(t *testing.T) {
		mustBeOK(t,
			`
			r: a2@remote b1@remote
			a2@remote: b  # this is actually "b2"
			b1@remote:
			b2@remote:
			`,
			[]string{
				"a2@remote",
				"b2@remote",
			},
		)
	})

	t.Run("Does not use unreferenced pkgs", func(t *testing.T) {
		mustBeOK(t,
			`
			r: a1@remote b1@remote  # b1 will actually be ignored, since a1 bumps it to b2
			a1@remote: b2@remote
			b1@remote: c1@remote
			b2@remote:  # doesn't reference c anymore at all
			c1@remote:  # will be unused
			`,
			[]string{
				"a1@remote",
				"b2@remote",
			},
		)
	})

	t.Run("Ambiguous remote package location", func(t *testing.T) {
		_, err := runSpec(t, ctx, `
			r: a1@remote1 b1@remote1
			b1@remote1: a2@remote2
			a1@remote1:
			a2@remote2:
		`)
		assert.That(t, err, should.ErrLike(`package "@a" is imported from multiple different repositories`))
	})

	t.Run("Remote package referencing back local package", func(t *testing.T) {
		_, err := runSpec(t, ctx, `
			r: a1@remote b1
			a1@remote: b1@local
			b1@local:
		`)
		assert.That(t, err, should.ErrLike(`package "@b" is imported from multiple different repositories`))
	})
}

////////////////////////////////////////////////////////////////////////////////

func prepRepos(t *testing.T, local map[string]string, remote map[string]string) (RepoManager, *LocalDiskRepo) {
	localRepo := &LocalDiskRepo{
		Root: prepDisk(t, local),
		Key:  RepoKey{Root: true},
	}
	return &PreconfiguredRepoManager{
		Repos: []Repo{localRepo},
		Other: &TestRepoManager{
			Root: prepDisk(t, remote),
		},
	}, localRepo
}

func runSpec(t *testing.T, ctx context.Context, spec string) ([]*Dep, error) {
	type pkgVer struct {
		pkg  string
		ver  string
		repo string
	}

	trimComment := func(line string) string {
		line, _, _ = strings.Cut(line, "#")
		return strings.TrimSpace(line)
	}

	// Takes "a2@repo" and converts it to pkgVer{...}.
	pkg := func(spec string) pkgVer {
		spec, repo, _ := strings.Cut(strings.TrimSpace(spec), "@")
		pkg := spec[:1] // e.g. "a"
		ver := spec[1:] // e.g. "2" or ""
		return pkgVer{pkg, ver, repo}
	}

	// Returns a path within the repository to given file.
	pkgPath := func(p pkgVer, rel ...string) string {
		return path.Join(append([]string{strings.TrimPrefix(p.pkg, "@")}, rel...)...)
	}

	root := pkgVer{}

	localFiles := map[string]string{}
	remoteFiles := map[string]string{}

	for _, line := range strings.Split(spec, "\n") {
		line = trimComment(line)
		if line == "" {
			continue
		}

		srcSpec, depsSpec, _ := strings.Cut(line, ":")
		srcSpec = strings.TrimSpace(srcSpec)
		depsSpec = strings.TrimSpace(depsSpec)

		var srcPkg pkgVer
		if root.pkg == "" {
			// This is root. The root is always local.
			srcPkg = pkg(srcSpec)
			srcPkg.repo = "local"
			srcPkg.ver = "0"
			root = srcPkg
		} else {
			// Non-root packages are remote by default.
			srcPkg = pkg(srcSpec)
			if srcPkg.repo == "" {
				srcPkg.repo = "remote"
			}
			if srcPkg.ver == "" {
				srcPkg.ver = "0"
			}
		}

		var deps []pkgVer
		if depsSpec != "" {
			for _, depSpec := range strings.Split(depsSpec, " ") {
				deps = append(deps, pkg(depSpec))
			}
		}

		// Synthesize the PACKAGE.star.
		var b strings.Builder
		fmt.Fprintf(&b, "pkg.declare(name=%q, lucicfg=\"1.2.3\")\n", "@"+srcPkg.pkg)
		for _, dep := range deps {
			var depSpec string
			if dep.repo == "" {
				depSpec = fmt.Sprintf("pkg.source.local(path=%q)", "../"+dep.pkg)
			} else {
				depSpec = fmt.Sprintf(
					"pkg.source.googlesource(host=%q, repo=%q, ref=%q, path=%q, revision=%q)",
					"ignored-host",
					dep.repo,
					"ignored-ref",
					pkgPath(dep),
					"v"+dep.ver,
				)
			}
			fmt.Fprintf(&b, "pkg.depend(name=%q, source=%s)\n", "@"+dep.pkg, depSpec)
		}
		pkgStar := b.String()

		// Write it into the appropriate repository (along with a probe file we'll
		// use to check what dependencies were picked in the end).
		stageFile := func(p, body string) {
			// Note that "local" repo maps to both a remote repo and a local disk.
			// This is needed to test remote deps referencing back local deps.
			remoteFiles[path.Join(srcPkg.repo, "v"+srcPkg.ver, pkgPath(srcPkg), p)] = body
			if srcPkg.repo == "local" {
				localFiles[path.Join(pkgPath(srcPkg), p)] = body
			}
		}
		pkgVerStr := fmt.Sprintf("%s%s@%s", srcPkg.pkg, srcPkg.ver, srcPkg.repo)
		stageFile("PACKAGE.star", pkgStar)
		stageFile("probe.star", pkgVerStr)
		t.Logf("%s PACKAGE.star:\n%s\n", pkgVerStr, pkgStar)
	}

	repos, local := prepRepos(t, localFiles, remoteFiles)

	rootDef, err := LoadDefinition(ctx,
		[]byte(localFiles[pkgPath(root, "PACKAGE.star")]),
		NoopLoaderValidator{},
	)
	if err != nil {
		return nil, err
	}

	return discoverDeps(ctx, &DepContext{
		Package:     rootDef.Name,
		Version:     PinnedVersion,
		Repo:        local,
		Path:        pkgPath(root),
		RepoManager: repos,
		Known:       rootDef,
	})
}

// probeVersions reads probe.star files created by runSpec.
func probeVersions(ctx context.Context, deps []*Dep) []string {
	var out []string
	for _, dep := range deps {
		_, probe, err := dep.Code(ctx, "probe.star")
		if err != nil {
			panic(err)
		}
		out = append(out, probe)
	}
	slices.Sort(out)
	return out
}
