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
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(treeEntry{}))
}

func TestCatFileTree(t *testing.T) {
	t.Parallel()

	r := mkRepo(t)

	got, err := r.batchProcLazy.catFileTree(context.Background(), "9c350ff8eeacef414a4baa5d68098e61b2d51a51", "")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(tree{
		"collision1":     {mode: 0o100644, kind: BlobKind, hash: "6371c00ed96239f955ecdb11c96fcd578eff821d"},
		"collision2":     {mode: 0o100644, kind: BlobKind, hash: "637172bb3ec0d2d76c7e5b2b5f21b9e5bf3aae96"},
		"subdir":         {mode: 0o40000, kind: TreeKind, hash: "af90ae6e2e62fde20aeefdec4c045682f00ef551"},
		"unrelated_file": {mode: 0o100644, kind: BlobKind, hash: "366cd2cbff6025c2ace384a7e43a5b81f29ebc75"},
	}))
}

func TestCatFileTreeRecursive(t *testing.T) {
	t.Parallel()

	r := mkRepo(t)

	got, err := r.batchProcLazy.catFileTreeRecursive(context.Background(), "9c350ff8eeacef414a4baa5d68098e61b2d51a51", "")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(tree{
		"collision1":                          {mode: 0o100644, kind: BlobKind, hash: "6371c00ed96239f955ecdb11c96fcd578eff821d"},
		"collision2":                          {mode: 0o100644, kind: BlobKind, hash: "637172bb3ec0d2d76c7e5b2b5f21b9e5bf3aae96"},
		"subdir":                              {mode: 0o40000, kind: TreeKind, hash: "af90ae6e2e62fde20aeefdec4c045682f00ef551"},
		"subdir/PACKAGE.star":                 {mode: 0o100644, kind: BlobKind, hash: "fa70b1cd267f69a7a6ea237364ae3b71907b5005"},
		"subdir/UNREFERENCED_DATA":            {mode: 0o100644, kind: BlobKind, hash: "0a2bb89ae387fbbfe5e2eecf5a1c49f45e30e5dc"},
		"subdir/builders.json":                {mode: 0o100644, kind: BlobKind, hash: "9acec8c9a4931845dea5669205e319498c787844"},
		"subdir/generated":                    {mode: 0o40000, kind: TreeKind, hash: "5dac1d6b9095a126188f69b2527625b00addbcea"},
		"subdir/generated/cr-buildbucket.cfg": {mode: 0o100644, kind: BlobKind, hash: "05531497834273d365cc3f1c318069ed0e222b9d"},
		"subdir/generated/project.cfg":        {mode: 0o100644, kind: BlobKind, hash: "3573c9a2ac92f0f96a5e53fa29c10e5fd09c983b"},
		"subdir/generated/realms.cfg":         {mode: 0o100644, kind: BlobKind, hash: "b423929a035c9209e85661588be48f304585b0bb"},
		"subdir/main.star":                    {mode: 0o100755, kind: BlobKind, hash: "37f1199eb96be259092fb9906d4d412ea7357f2d"},
		"unrelated_file":                      {mode: 0o100644, kind: BlobKind, hash: "366cd2cbff6025c2ace384a7e43a5b81f29ebc75"},
	}))
}
