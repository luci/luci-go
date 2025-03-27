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
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/lucicfg/fileset"
)

func TestDiskPackageLoader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tmp := prepDisk(t, map[string]string{
		"PACKAGE.star":         "The main root",
		"1.star":               "1",
		"good/2.star":          "2",
		"good/as/well/3.star":  "3",
		"good/res.txt":         "zzz",
		"good/as/well/res.txt": "zzz",
		"bad/PACKAGE.star":     "Nested package",
		"bad/4.star":           "4",
		"bad/as/well/5.star":   "5",
		"wrong/res.bin":        "zzz",
	})

	resources, err := fileset.New([]string{"**/*.txt"})
	assert.NoErr(t, err)

	loader, err := diskPackageLoader(tmp, "@pkg", resources, syncStatCache())
	assert.NoErr(t, err)

	good := []struct {
		path string
		body string
	}{
		{"1.star", "1"},
		{"good/2.star", "2"},
		{"good/as/well/3.star", "3"},
		{"good/res.txt", "zzz"},
		{"good/as/well/res.txt", "zzz"},
	}
	for _, cs := range good {
		_, body, err := loader(ctx, cs.path)
		assert.NoErr(t, err)
		assert.That(t, body, should.Equal(cs.body))
	}

	bad := []struct {
		path string
		err  string
	}{
		{".", "outside the package root"},
		{"../file.star", "outside the package root"},
		{"bad/4.star", `directory "bad" belongs to a different (nested) package and files from it cannot be loaded directly`},
		{"bad/as/well/5.star", `directory "bad/as/well" belongs to a different (nested) package and files from it cannot be loaded directly`},
		{"wrong/res.bin", `this non-starlark file is not declared as a resource in pkg.resources(...) in PACKAGE.star of "@pkg" and cannot be loaded`},
		{"missing.star", `no such module`},
	}
	for _, cs := range bad {
		_, _, err := loader(ctx, cs.path)
		assert.That(t, err, should.ErrLike(cs.err))
	}
}
