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

	"go.chromium.org/luci/common/testing/testfs"

	"go.chromium.org/luci/vpython/api/vpython"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey(`Test LoadForScript`, t, func() {
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

		Convey(`Layout: module with a good spec file`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/bar.vpython":         goodSpecData,
			})
			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("foo/bar.vpython"))
		})

		Convey(`Layout: module with a bad spec file`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/bar.vpython":         badSpecData,
			})
			_, _, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldErrLike, "failed to unmarshal vpython.Spec")
		})

		Convey(`Layout: module with no spec file`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/__init__.py":         "",
			})
			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldBeNil)
			So(spec, ShouldBeNil)
			So(path, ShouldEqual, "")
		})

		Convey(`Layout: individual file with a good spec file`, func() {
			mustBuild(map[string]string{
				"pants.py":         "PANTS!",
				"pants.py.vpython": goodSpecData,
			})
			spec, path, err := l.LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("pants.py.vpython"))
		})

		Convey(`Layout: individual file with a bad spec file`, func() {
			mustBuild(map[string]string{
				"pants.py":         "PANTS!",
				"pants.py.vpython": badSpecData,
			})
			_, _, err := l.LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldErrLike, "failed to unmarshal vpython.Spec")
		})

		Convey(`Layout: individual file with no spec (inline or file)`, func() {
			mustBuild(map[string]string{
				"pants.py": "PANTS!",
			})
			spec, path, err := l.LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldBeNil)
			So(path, ShouldBeEmpty)
		})

		Convey(`Layout: module with good inline spec`, func() {
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
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("foo/bar/baz/__main__.py"))
		})

		Convey(`Layout: individual file with good inline spec`, func() {
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
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("pants.py"))
		})

		Convey(`Layout: individual file with good inline spec with a prefix`, func() {
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
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("pants.py"))
		})

		Convey(`Layout: individual file with bad inline spec`, func() {
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
			So(err, ShouldErrLike, "failed to unmarshal vpython.Spec")
		})

		Convey(`Layout: individual file with inline spec, missing end barrier`, func() {
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
			So(err, ShouldErrLike, "unterminated inline spec file")
		})

		Convey(`Layout: individual file with a common spec`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"common.vpython":      goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("common.vpython"))
		})

		Convey(`Layout: individual file with a custom common spec`, func() {
			l.CommonSpecNames = []string{"ohaithere.friend", "common.vpython"}

			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"common.vpython":      badSpecData,
				"ohaithere.friend":    goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("ohaithere.friend"))
		})

		Convey(`Layout: individual file with a common spec behind a barrier`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"foo/BARRIER":         "",
				"common.vpython":      goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldBeNil)
			So(path, ShouldBeEmpty)
		})

		Convey(`Layout: individual file with a spec and barrier sharing a folder`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"BARRIER":             "",
				"common.vpython":      goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("common.vpython"))
		})

		Convey(`Layout: module with a common spec`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"common.vpython":          goodSpecData,
			})

			spec, path, err := l.LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldBeNil)
			So(spec, ShouldResembleProto, goodSpec)
			So(path, ShouldEqual, makePath("common.vpython"))
		})

		Convey(`Layout: individual file with a bad common spec`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz.py":      "main",
				"foo/bar/__init__.py": "",
				"common.vpython":      badSpecData,
			})

			_, _, err := l.LoadForScript(c, makePath("foo/bar/baz.py"), false)
			So(err, ShouldErrLike, "failed to unmarshal vpython.Spec")
		})
	})
}
