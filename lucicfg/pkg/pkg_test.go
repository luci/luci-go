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
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestEntryOnDisk(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("Legacy mode", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config":     `# Denotes repo root`,
			"a/b/c/main.star": `print("Hi")`,
		})

		entry, err := EntryOnDisk(ctx, filepath.Join(tmp, "a/b/c/main.star"))
		assert.NoErr(t, err)
		assert.That(t, entry.Local.DiskPath, should.Equal(filepath.Join(tmp, "a/b/c")))

		_, src, err := entry.Main(ctx, "main.star")
		assert.NoErr(t, err)
		assert.That(t, src, should.Equal(`print("Hi")`))

		assert.Loosely(t, entry.Deps, should.HaveLength(0))
		assert.That(t, entry.Path, should.Equal("a/b/c"))
		assert.That(t, entry.Script, should.Equal("main.star"))
		assert.Loosely(t, entry.LucicfgVersionConstraints, should.HaveLength(0))
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

		entry, err := EntryOnDisk(ctx, filepath.Join(tmp, "a/b/c/main.star"))
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
	})

	t.Run("Borked PACKAGE.star", func(t *testing.T) {
		tmp := prepDisk(t, map[string]string{
			".git/config":  `# Denotes repo root`,
			"PACKAGE.star": ``,
		})
		_, err := EntryOnDisk(ctx, filepath.Join(tmp, "main.star"))
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
		_, err := EntryOnDisk(ctx, filepath.Join(tmp, "missing.star"))
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
		_, err := EntryOnDisk(ctx, filepath.Join(tmp, "main.star"))
		assert.That(t, err, should.ErrLike(
			`main.star is not declared as a pkg.entrypoint(...) in PACKAGE.star and thus cannot be executed. Available entrypoints: [another1.star another2.star]`))
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

func prepDisk(t *testing.T, files map[string]string) string {
	tmp := t.TempDir()
	for path, body := range files {
		abs := filepath.Join(tmp, filepath.FromSlash(path))
		assert.NoErr(t, os.MkdirAll(filepath.Dir(abs), 0750))
		assert.NoErr(t, os.WriteFile(abs, []byte(deindent(body)), 0660))
	}
	return tmp
}
