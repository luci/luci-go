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
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/lucicfg/lockfilepb"
)

func TestCheckLockfileStaleness(t *testing.T) {
	t.Parallel()

	mockLockfile := func(pkgName, lucicfgVer string) *lockfilepb.Lockfile {
		return &lockfilepb.Lockfile{
			Lucicfg: lucicfgVer,
			Packages: []*lockfilepb.Lockfile_Package{
				{
					Name:        pkgName,
					Deps:        []string{"@pkg1", "@pkg2"},
					Lucicfg:     "1.2.3",
					Resources:   []string{"a", "b", "c"},
					Entrypoints: []string{"a"},
				},
			},
		}
	}

	prepMock := func(pkgName, lucicfgVer string) string {
		blob, err := serializeLockfile(mockLockfile(pkgName, lucicfgVer))
		assert.NoErr(t, err)
		return string(blob)
	}

	p := prepDisk(t, map[string]string{
		"identical/PACKAGE.lock": prepMock("@test", "1.2.3"),
		"semeq/PACKAGE.lock":     prepMock("@test", "4.5.6"),
		"another/PACKAGE.lock":   prepMock("@another", "1.2.3"),
		"broken/PACKAGE.lock":    "NOT JSONPB",
		"missing/.ignore":        "",
	})

	canonical := mockLockfile("@test", "1.2.3")

	t.Run("semanticEq = false", func(t *testing.T) {
		cases := []struct {
			path  string
			fresh bool
		}{
			{"identical", true},
			{"semeq", false},
			{"another", false},
			{"broken", false},
			{"missing", false},
		}
		for _, cs := range cases {
			fresh, err := CheckLockfileStaleness(filepath.Join(p, cs.path), canonical, false)
			assert.NoErr(t, err)
			assert.That(t, fresh, should.Equal(cs.fresh))
		}
	})

	t.Run("semanticEq = true", func(t *testing.T) {
		cases := []struct {
			path  string
			fresh bool
		}{
			{"identical", true},
			{"semeq", true},
			{"another", false},
			{"broken", false},
			{"missing", false},
		}
		for _, cs := range cases {
			fresh, err := CheckLockfileStaleness(filepath.Join(p, cs.path), canonical, true)
			assert.NoErr(t, err)
			assert.That(t, fresh, should.Equal(cs.fresh))
		}
	})
}

func TestSerializeLockfile(t *testing.T) {
	t.Parallel()

	lockfile := &lockfilepb.Lockfile{
		Lucicfg: "1.2.3",
		Packages: []*lockfilepb.Lockfile_Package{
			{
				Name:        "@test",
				Deps:        []string{"@pkg1", "@pkg2"},
				Lucicfg:     "1.2.3",
				Resources:   []string{"a", "b", "c"},
				Entrypoints: []string{"a"},
			},
		},
	}

	blob, err := serializeLockfile(lockfile)
	assert.NoErr(t, err)
	assert.That(t, string(blob), should.Equal(`{
	"lucicfg": "1.2.3",
	"packages": [
		{
			"name": "@test",
			"deps": [
				"@pkg1",
				"@pkg2"
			],
			"lucicfg": "1.2.3",
			"resources": [
				"a",
				"b",
				"c"
			],
			"entrypoints": [
				"a"
			]
		}
	]
}
`))
}
