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

package gitsource

import (
	"context"
	"fmt"
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucicfg/pkg/source"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(treeEntry{}))
}

func TestCatFileTree(t *testing.T) {
	t.Parallel()

	commit := "628403cb1e193d0131c39394dfd6abed938583a7"

	r := mkRepo(t, commit+":") // ":" gets the tree

	got, err := r.batchProc.catFileTree(context.Background(), commit, "")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(tree{
		{"collision1", 0o100644, source.BlobKind, "6371c00ed96239f955ecdb11c96fcd578eff821d"},
		{"collision2", 0o100644, source.BlobKind, "637172bb3ec0d2d76c7e5b2b5f21b9e5bf3aae96"},
		{"infra", 0o160000, source.GitLinkKind, "4585ca362480300618eec126795cb0572a0adda8"},
		{"similardirs", 0o40000, source.TreeKind, "55fa4e4b59d7d03fc048bce7d64176d889a6e49b"},
		{"subdir", 0o40000, source.TreeKind, "af90ae6e2e62fde20aeefdec4c045682f00ef551"},
		{"unrelated_file", 0o100644, source.BlobKind, "366cd2cbff6025c2ace384a7e43a5b81f29ebc75"},
	}))
}

func TestCatFileTreeWalk(t *testing.T) {
	t.Parallel()

	commit := "9c350ff8eeacef414a4baa5d68098e61b2d51a51"

	r := mkRepo(t, commit+":")

	var wholeTree tree

	err := r.batchProc.catFileTreeWalk(context.Background(), commit, "", nil, func(repoRelPath string, dirContent tree) (map[int]struct{}, error) {
		for _, entry := range dirContent {
			wholeTree = append(wholeTree, entry)
			wholeTree[len(wholeTree)-1].name = path.Join(repoRelPath, entry.name)
		}
		return nil, nil
	})
	assert.NoErr(t, err)
	assert.That(t, wholeTree, should.Match(tree{
		{"collision1", 0o100644, source.BlobKind, "6371c00ed96239f955ecdb11c96fcd578eff821d"},
		{"collision2", 0o100644, source.BlobKind, "637172bb3ec0d2d76c7e5b2b5f21b9e5bf3aae96"},
		{"subdir", 0o40000, source.TreeKind, "af90ae6e2e62fde20aeefdec4c045682f00ef551"},
		{"unrelated_file", 0o100644, source.BlobKind, "366cd2cbff6025c2ace384a7e43a5b81f29ebc75"},
		{"subdir/PACKAGE.star", 0o100644, source.BlobKind, "fa70b1cd267f69a7a6ea237364ae3b71907b5005"},
		{"subdir/UNREFERENCED_DATA", 0o100644, source.BlobKind, "0a2bb89ae387fbbfe5e2eecf5a1c49f45e30e5dc"},
		{"subdir/builders.json", 0o100644, source.BlobKind, "9acec8c9a4931845dea5669205e319498c787844"},
		{"subdir/generated", 0o40000, source.TreeKind, "5dac1d6b9095a126188f69b2527625b00addbcea"},
		{"subdir/main.star", 0o100755, source.BlobKind, "37f1199eb96be259092fb9906d4d412ea7357f2d"},
		{"subdir/generated/cr-buildbucket.cfg", 0o100644, source.BlobKind, "05531497834273d365cc3f1c318069ed0e222b9d"},
		{"subdir/generated/project.cfg", 0o100644, source.BlobKind, "3573c9a2ac92f0f96a5e53fa29c10e5fd09c983b"},
		{"subdir/generated/realms.cfg", 0o100644, source.BlobKind, "b423929a035c9209e85661588be48f304585b0bb"},
	}))
}

func TestCatFileTreeWalkSkipTree(t *testing.T) {
	t.Parallel()

	commit := "9c350ff8eeacef414a4baa5d68098e61b2d51a51"

	r := mkRepo(t, commit+":")

	var wholeTree tree

	err := r.batchProc.catFileTreeWalk(context.Background(), commit, "", nil, func(repoRelPath string, dirContent tree) (map[int]struct{}, error) {
		skips := map[int]struct{}{}
		for i, entry := range dirContent {
			if entry.name == "generated" {
				skips[i] = struct{}{}
			}
			wholeTree = append(wholeTree, entry)
			wholeTree[len(wholeTree)-1].name = path.Join(repoRelPath, entry.name)
		}
		return skips, nil
	})
	assert.NoErr(t, err)
	assert.That(t, wholeTree, should.Match(tree{
		{"collision1", 0o100644, source.BlobKind, "6371c00ed96239f955ecdb11c96fcd578eff821d"},
		{"collision2", 0o100644, source.BlobKind, "637172bb3ec0d2d76c7e5b2b5f21b9e5bf3aae96"},
		{"subdir", 0o40000, source.TreeKind, "af90ae6e2e62fde20aeefdec4c045682f00ef551"},
		{"unrelated_file", 0o100644, source.BlobKind, "366cd2cbff6025c2ace384a7e43a5b81f29ebc75"},
		{"subdir/PACKAGE.star", 0o100644, source.BlobKind, "fa70b1cd267f69a7a6ea237364ae3b71907b5005"},
		{"subdir/UNREFERENCED_DATA", 0o100644, source.BlobKind, "0a2bb89ae387fbbfe5e2eecf5a1c49f45e30e5dc"},
		{"subdir/builders.json", 0o100644, source.BlobKind, "9acec8c9a4931845dea5669205e319498c787844"},
		{"subdir/generated", 0o40000, source.TreeKind, "5dac1d6b9095a126188f69b2527625b00addbcea"},
		{"subdir/main.star", 0o100755, source.BlobKind, "37f1199eb96be259092fb9906d4d412ea7357f2d"},
	}))
}

func TestCatFileTreeWalkSameTree(t *testing.T) {
	t.Parallel()

	commit := "a03999f1cc369f7cbbea98a4027eee27f642098d"
	subdir := "similardirs"

	r := mkRepo(t, fmt.Sprintf("%s:%s", commit, subdir))

	var wholeTree tree

	err := r.batchProc.catFileTreeWalk(context.Background(), commit, subdir, nil, func(repoRelPath string, dirContent tree) (map[int]struct{}, error) {
		for _, entry := range dirContent {
			wholeTree = append(wholeTree, entry)
			wholeTree[len(wholeTree)-1].name = path.Join(repoRelPath, entry.name)
		}
		return nil, nil
	})
	assert.NoErr(t, err)

	// note that `a` and `b` have the same tree hash deb57a1a256b3fb1e0725fbf42fee8d047f72293
	assert.That(t, wholeTree, should.Match(tree{
		{"similardirs/a", 0o40000, source.TreeKind, "deb57a1a256b3fb1e0725fbf42fee8d047f72293"},
		{"similardirs/b", 0o40000, source.TreeKind, "deb57a1a256b3fb1e0725fbf42fee8d047f72293"},
		{"similardirs/a/foo.json", 0o100644, source.BlobKind, "35e752b3afcf6391fdbd17f25285d9963f349b50"},
		{"similardirs/b/foo.json", 0o100644, source.BlobKind, "35e752b3afcf6391fdbd17f25285d9963f349b50"},
	}))
}
