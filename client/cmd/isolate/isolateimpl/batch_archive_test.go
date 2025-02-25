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

package isolateimpl

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.chromium.org/luci/common/flag/stringmapflag"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConvertPyToGoArchiveCMDArgs(t *testing.T) {
	ftt.Run(`Archive command line arguments should be converted properly for Go.`, t, func(t *ftt.Test) {
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
			assert.Loosely(t, convertPyToGoArchiveCMDArgs(line.input), should.Match(line.expected))
		}
	})
}

func TestInvalidArchiveCMD(t *testing.T) {
	ftt.Run(`Archive should handle errors in command line arguments.`, t, func(t *ftt.Test) {
		_, err := parseArchiveCMD([]string(nil), absToOS("e:", "/tmp/bar"))
		assert.Loosely(t, err.Error(), should.Match("-isolate must be specified"))
	})
}

func TestArchiveCMDParsing(t *testing.T) {
	ftt.Run(`Archive command line arguments should be parsed correctly.`, t, func(t *ftt.Test) {
		args := []string{
			"--isolate", "../boz/bar.isolate",
			"--path-variable", "DEPTH", "../..",
			"--path-variable", "PRODUCT_DIR", "../../out/Release",
			"--config-variable", "OS=linux",
		}
		root := absToOS("e:", "/tmp/bar")
		opts, err := parseArchiveCMD(args, root)
		base := filepath.Dir(root)
		assert.Loosely(t, opts.Isolate, should.Match(filepath.Join(base, "boz", "bar.isolate")))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, stringmapflag.Value{"OS": "linux"}, should.Match(opts.ConfigVariables))
		if runtime.GOOS == "windows" {
			assert.Loosely(t, stringmapflag.Value{"PRODUCT_DIR": "../../out/Release", "EXECUTABLE_SUFFIX": ".exe", "DEPTH": "../.."}, should.Match(opts.PathVariables))
		} else {
			assert.Loosely(t, stringmapflag.Value{"PRODUCT_DIR": "../../out/Release", "EXECUTABLE_SUFFIX": "", "DEPTH": "../.."}, should.Match(opts.PathVariables))
		}
	})
}

// Verify that if the isolate path is absolute, we don't
// accidentally interpret them as relative to the cwd.
func TestArchiveAbsolutePaths(t *testing.T) {
	ftt.Run(`Archive command line should correctly handle absolute paths.`, t, func(t *ftt.Test) {
		root := absToOS("e:", "/tmp/bar/")
		args := []string{
			"--isolate", root + "foo.isolate",
		}
		opts, err := parseArchiveCMD(args, absToOS("x:", "/var/lib"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, opts.Isolate, should.Match(root+"foo.isolate"))
	})
}

// Private stuff.

// absToOS converts a POSIX path to OS specific format.
func absToOS(drive, p string) string {
	if runtime.GOOS == "windows" {
		return drive + strings.Replace(p, "/", "\\", -1)
	}
	return p
}
