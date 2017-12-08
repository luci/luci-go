// Copyright 2015 The LUCI Authors.
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

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/flag/stringmapflag"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConvertPyToGoArchiveCMDArgs(t *testing.T) {
	t.Parallel()
	Convey(`Archive command line arguments should be converted properly for Go.`, t, func() {
		data := []struct {
			input    []string
			expected []string
		}{
			// Simple.
			{
				[]string{"--path-variable", "key=value"},
				[]string{"--path-variable", "key=value"},
			},
			{
				[]string{"--path-variable", "key", "value1"},
				[]string{"--path-variable", "key=value1"},
			},
			// That's how python isolate works.
			{
				[]string{"--extra-variable", "key", "and spaces"},
				[]string{"--extra-variable", "key=and spaces"},
			},
			{
				[]string{"--path-variable", "key", "--even-this-value"},
				[]string{"--path-variable", "key=--even-this-value"},
			},
			// Other args.
			{
				[]string{"-x", "--var", "--config-variable", "key", "value"},
				[]string{"-x", "--var", "--config-variable", "key=value"},
			},
			{
				[]string{"--path-variable", "key", "value", "posarg"},
				[]string{"--path-variable", "key=value", "posarg"},
			},
			// Too few args are just ignored.
			{
				[]string{"--path-variable"},
				[]string{"--path-variable"},
			},
			{
				[]string{"--path-variable", "key-and-no-value"},
				[]string{"--path-variable", "key-and-no-value"},
			},
		}
		for _, line := range data {
			So(convertPyToGoArchiveCMDArgs(line.input), ShouldResemble, line.expected)
		}
	})
}

func TestInvalidArchiveCMD(t *testing.T) {
	t.Parallel()
	Convey(`Archive should handle errors in command line argments.`, t, func() {
		_, err := parseArchiveCMD([]string(nil), absToOS("e:", "/tmp/bar"))
		So(err.Error(), ShouldResemble, "-isolated must be specified")
	})
}

func TestArchiveCMDParsing(t *testing.T) {
	t.Parallel()
	Convey(`Archive command line arguments should be parsed correctly.`, t, func() {
		args := []string{
			"--isolated", "../biz/bar.isolated",
			"--isolate", "../boz/bar.isolate",
			"--path-variable", "DEPTH", "../..",
			"--path-variable", "PRODUCT_DIR", "../../out/Release",
			"--extra-variable", "version_full=42.0.2284.0",
			"--config-variable", "OS=linux",
		}
		root := absToOS("e:", "/tmp/bar")
		opts, err := parseArchiveCMD(args, root)
		base := filepath.Dir(root)
		So(opts.Isolate, ShouldResemble, filepath.Join(base, "boz", "bar.isolate"))
		So(opts.Isolated, ShouldResemble, filepath.Join(base, "biz", "bar.isolated"))
		So(err, ShouldBeNil)
		So(stringmapflag.Value{"OS": "linux"}, ShouldResemble, opts.ConfigVariables)
		if common.IsWindows() {
			So(stringmapflag.Value{"PRODUCT_DIR": "../../out/Release", "EXECUTABLE_SUFFIX": ".exe", "DEPTH": "../.."}, ShouldResemble, opts.PathVariables)
		} else {
			So(stringmapflag.Value{"PRODUCT_DIR": "../../out/Release", "EXECUTABLE_SUFFIX": "", "DEPTH": "../.."}, ShouldResemble, opts.PathVariables)
		}
		So(stringmapflag.Value{"version_full": "42.0.2284.0"}, ShouldResemble, opts.ExtraVariables)
	})
}

// Verify that if the isolate/isolated paths are absolute, we don't
// accidentally interpret them as relative to the cwd.
func TestArchiveAbsolutePaths(t *testing.T) {
	t.Parallel()
	Convey(`Archive command line should correctly handle absolute paths.`, t, func() {
		root := absToOS("e:", "/tmp/bar/")
		args := []string{
			"--isolated", root + "foo.isolated",
			"--isolate", root + "foo.isolate",
		}
		opts, err := parseArchiveCMD(args, absToOS("x:", "/var/lib"))
		So(err, ShouldBeNil)
		So(opts.Isolate, ShouldResemble, root+"foo.isolate")
		So(opts.Isolated, ShouldResemble, root+"foo.isolated")
	})
}

// Private stuff.

// absToOS converts a POSIX path to OS specific format.
func absToOS(drive, p string) string {
	if common.IsWindows() {
		return drive + strings.Replace(p, "/", "\\", -1)
	}
	return p
}
