// Copyright 2017 The LUCI Authors.
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

package spec

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/testfs"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/api/vpython"
)

func TestLoadForScript(t *testing.T) {
	t.Parallel()

	goodSpec := &vpython.Spec{
		PythonVersion: "3.4.0",
		Wheel: []*vpython.Spec_Package{
			{Name: "foo/bar", Version: "1"},
			{Name: "baz/qux", Version: "2"},
		},
	}
	goodSpecData := prototext.Format(goodSpec)
	badSpecData := "foo: bar"

	l := Loader{
		CommonFilesystemBarriers: []string{"BARRIER"},
	}

	ftt.Run(`Test LoadForScript`, t, func(t *ftt.Test) {
		tdir := t.TempDir()
		c := context.Background()

		// On OSX and Linux the temp dir may be in a symlinked directory (and on OSX
		// it most often is). LoadForScript expands symlinks, so we need to expand
		// them here too to make path string comparisons below pass.
		if runtime.GOOS != "windows" {
			var err error
			if tdir, err = filepath.EvalSymlinks(tdir); err != nil {
				panic(err)
			}
		}

		makePath := func(path string) string {
			return filepath.Join(tdir, filepath.FromSlash(path))
		}
		mustBuild := func(layout map[string]string) {
			if err := testfs.Build(tdir, layout); err != nil {
				panic(err)
			}
		}

		t.Run(`Layout: module with a good spec file`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/bar.vpython":         goodSpecData,
			})
			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("foo/bar.vpython")))
		})

		t.Run(`Layout: module with a bad spec file`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/bar.vpython":         badSpecData,
			})
			_, _, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			assert.Loosely(t, err, should.ErrLike("failed to unmarshal vpython.Spec"))
		})

		t.Run(`Layout: module with no spec file`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/__init__.py":         "",
			})
			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.BeNil)
			assert.Loosely(t, path, should.BeEmpty)
		})

		t.Run(`Layout: individual file with a good spec file`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"pants.py":         "PANTS!",
				"pants.py.vpython": goodSpecData,
			})
			spec, path, err := l.LoadForScript(c, makePath("pants.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("pants.py.vpython")))
		})

		t.Run(`Layout: individual file with a bad spec file`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"pants.py":         "PANTS!",
				"pants.py.vpython": badSpecData,
			})
			_, _, err := l.LoadForScript(c, makePath("pants.py"), false)
			assert.Loosely(t, err, should.ErrLike("failed to unmarshal vpython.Spec"))
		})

		t.Run(`Layout: individual file with no spec (inline or file)`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"pants.py": "PANTS!",
			})
			spec, path, err := l.LoadForScript(c, makePath("pants.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.BeNil)
			assert.Loosely(t, path, should.BeEmpty)
		})

		t.Run(`Layout: module with good inline spec`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": strings.Join([]string{
					"#!/usr/bin/env vpython",
					"",
					"# Test file",
					"",
					`"""Docstring`,
					"[VPYTHON:BEGIN]",
					goodSpecData,
					"[VPYTHON:END]",
					`"""`,
					"",
					"# Additional content...",
				}, "\n"),
			})
			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("foo/bar/baz/__main__.py")))
		})

		t.Run(`Layout: individual file with good inline spec`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"pants.py": strings.Join([]string{
					"#!/usr/bin/env vpython",
					"",
					"# Test file",
					"",
					`"""Docstring`,
					"[VPYTHON:BEGIN]",
					goodSpecData,
					"[VPYTHON:END]",
					`"""`,
					"",
					"# Additional content...",
				}, "\n"),
			})
			spec, path, err := l.LoadForScript(c, makePath("pants.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("pants.py")))
		})

		t.Run(`Layout: individual file with good inline spec with a prefix`, func(t *ftt.Test) {
			specParts := strings.Split(goodSpecData, "\n")
			for i, line := range specParts {
				specParts[i] = strings.TrimSpace("# " + line)
			}

			mustBuild(map[string]string{
				"pants.py": strings.Join([]string{
					"#!/usr/bin/env vpython",
					"",
					"# Test file",
					"#",
					"# [VPYTHON:BEGIN]",
					strings.Join(specParts, "\n"),
					"# [VPYTHON:END]",
					"",
					"# Additional content...",
				}, "\n"),
			})

			spec, path, err := l.LoadForScript(c, makePath("pants.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("pants.py")))
		})

		t.Run(`Layout: individual file with bad inline spec`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"pants.py": strings.Join([]string{
					"#!/usr/bin/env vpython",
					"",
					"# Test file",
					"",
					`"""Docstring`,
					"[VPYTHON:BEGIN]",
					badSpecData,
					"[VPYTHON:END]",
					`"""`,
					"",
					"# Additional content...",
				}, "\n"),
			})

			_, _, err := l.LoadForScript(c, makePath("pants.py"), false)
			assert.Loosely(t, err, should.ErrLike("failed to unmarshal vpython.Spec"))
		})

		t.Run(`Layout: individual file with inline spec, missing end barrier`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"pants.py": strings.Join([]string{
					"#!/usr/bin/env vpython",
					"",
					"# Test file",
					"",
					`"""Docstring`,
					"[VPYTHON:BEGIN]",
					goodSpecData,
					`"""`,
					"",
					"# Additional content...",
				}, "\n"),
			})

			_, _, err := l.LoadForScript(c, makePath("pants.py"), false)
			assert.Loosely(t, err, should.ErrLike("unterminated inline spec file"))
		})

		t.Run(`Layout: individual file with a common spec`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"common.vpython":      goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("common.vpython")))
		})

		t.Run(`Layout: individual file with a custom common spec`, func(t *ftt.Test) {
			l.CommonSpecNames = []string{"ohaithere.friend", "common.vpython"}

			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"common.vpython":      badSpecData,
				"ohaithere.friend":    goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("ohaithere.friend")))
		})

		t.Run(`Layout: individual file with a common spec behind a barrier`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"foo/BARRIER":         "",
				"common.vpython":      goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.BeNil)
			assert.Loosely(t, path, should.BeEmpty)
		})

		t.Run(`Layout: individual file with a spec and barrier sharing a folder`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"BARRIER":             "",
				"common.vpython":      goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("common.vpython")))
		})

		t.Run(`Layout: module with a common spec`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"common.vpython":          goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, spec, should.Match(goodSpec))
			assert.Loosely(t, path, should.Equal(makePath("common.vpython")))
		})

		t.Run(`Layout: individual file with a bad common spec`, func(t *ftt.Test) {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"common.vpython":      badSpecData,
			})

			_, _, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			assert.Loosely(t, err, should.ErrLike("failed to unmarshal vpython.Spec"))
		})
	})
}
