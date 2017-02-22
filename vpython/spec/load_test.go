// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package spec

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/filesystem/testfs"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadForScript(t *testing.T) {
	t.Parallel()

	goodSpec := &vpython.Spec{
		PythonVersion: "3.4.0",
		Wheel: []*vpython.Spec_Package{
			{Path: "foo/bar", Version: "1"},
			{Path: "baz/qux", Version: "2"},
		},
	}
	goodSpecData := proto.MarshalTextString(goodSpec)
	badSpecData := "foo: bar"

	Convey(`Test LoadForScript`, t, testfs.MustWithTempDir(t, "TestLoadForScript", func(tdir string) {
		c := context.Background()

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
			spec, err := LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldBeNil)
			So(spec, ShouldResemble, goodSpec)
		})

		Convey(`Layout: module with a bad spec file`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/bar.vpython":         badSpecData,
			})
			_, err := LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldErrLike, "failed to unmarshal vpython.Spec")
		})

		Convey(`Layout: module with no spec file`, func() {
			mustBuild(map[string]string{
				"foo/bar/baz/__main__.py": "main",
				"foo/bar/baz/__init__.py": "",
				"foo/bar/__init__.py":     "",
				"foo/__init__.py":         "",
			})
			spec, err := LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldBeNil)
			So(spec, ShouldBeNil)
		})

		Convey(`Layout: individual file with a good spec file`, func() {
			mustBuild(map[string]string{
				"pants.py":         "PANTS!",
				"pants.py.vpython": goodSpecData,
			})
			spec, err := LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldResemble, goodSpec)
		})

		Convey(`Layout: individual file with a bad spec file`, func() {
			mustBuild(map[string]string{
				"pants.py":         "PANTS!",
				"pants.py.vpython": badSpecData,
			})
			_, err := LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldErrLike, "failed to unmarshal vpython.Spec")
		})

		Convey(`Layout: individual file with no spec (inline or file)`, func() {
			mustBuild(map[string]string{
				"pants.py": "PANTS!",
			})
			spec, err := LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldBeNil)
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
			spec, err := LoadForScript(c, makePath("foo/bar/baz"), true)
			So(err, ShouldBeNil)
			So(spec, ShouldResemble, goodSpec)
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
			spec, err := LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldResemble, goodSpec)
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

			spec, err := LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldBeNil)
			So(spec, ShouldResemble, goodSpec)
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

			_, err := LoadForScript(c, makePath("pants.py"), false)
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

			_, err := LoadForScript(c, makePath("pants.py"), false)
			So(err, ShouldErrLike, "unterminated inline spec file")
		})
	}))
}
