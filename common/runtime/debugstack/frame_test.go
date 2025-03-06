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

package debugstack

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSourceInfo(t *testing.T) {
	t.Parallel()

	type tcase struct {
		stack  string
		path   string
		lineno int
	}

	run := func(tc tcase) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()

			parsed := ParseString(tc.stack)
			path, lineno := parsed[0].SourceInfo()
			check.That(t, path, should.Equal(tc.path))
			check.That(t, lineno, should.Equal(tc.lineno))
		}
	}

	t.Run(`goroutine`, run(tcase{
		"goroutine 23 [running]\n",
		"",
		-1,
	}))

	t.Run(`normal`, run(tcase{
		"some/pkg.AFunc(0x0)\n\t/some/path/to/a/file.go:123 +0x8c",
		"/some/path/to/a/file.go",
		123,
	}))

	t.Run(`no lineno`, run(tcase{
		"some/pkg.AFunc(0x0)\n\t/some/path/to/a/file.go",
		"/some/path/to/a/file.go",
		0,
	}))

	t.Run(`no lineno w/ :`, run(tcase{
		"some/pkg.AFunc(0x0)\n\t/some/path/to/a/file.go:",
		"/some/path/to/a/file.go",
		0,
	}))

	t.Run(`no PC`, run(tcase{
		"some/pkg.AFunc(0x0)\n\t/some/path/to/a/file.go:321",
		"/some/path/to/a/file.go",
		321,
	}))

	t.Run(`windows`, run(tcase{
		"some/pkg.AFunc(0x0)\n\tC:/some/path/to/a/file.go:678 +0x9f",
		"C:/some/path/to/a/file.go",
		678,
	}))
}
