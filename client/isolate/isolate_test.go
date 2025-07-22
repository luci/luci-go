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

package isolate

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	log.SetOutput(io.Discard)
}

func TestReplaceVars(t *testing.T) {
	t.Parallel()
	ftt.Run(`Variables replacement should be supported in isolate files.`, t, func(t *ftt.Test) {

		opts := &ArchiveOptions{PathVariables: map[string]string{"VAR": "wonderful"}}

		// Single replacement.
		r, err := ReplaceVariables("hello <(VAR) world", opts)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, r, should.Match("hello wonderful world"))

		// Multiple replacement.
		r, err = ReplaceVariables("hello <(VAR) <(VAR) world", opts)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, r, should.Match("hello wonderful wonderful world"))

		// Replacement of missing variable.
		r, err = ReplaceVariables("hello <(MISSING) world", opts) // nolint:ineffassign
		assert.Loosely(t, err.Error(), should.Match("no value for variable 'MISSING'"))
	})
}

func TestIgnoredPathsRegexp(t *testing.T) {
	t.Parallel()

	ftt.Run(`Ignored file extensions`, t, func(t *ftt.Test) {
		regexStr := genExtensionsRegex("pyc", "swp")
		re, err := regexp.Compile(regexStr)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, re.MatchString("a.pyc"), should.BeTrue)
		assert.Loosely(t, re.MatchString("foo/a.pyc"), should.BeTrue)
		assert.Loosely(t, re.MatchString("/b.swp"), should.BeTrue)
		assert.Loosely(t, re.MatchString(`foo\b.swp`), should.BeTrue)

		assert.Loosely(t, re.MatchString("a.py"), should.BeFalse)
		assert.Loosely(t, re.MatchString("b.swppp"), should.BeFalse)
	})

	ftt.Run(`Ignored directories`, t, func(t *ftt.Test) {
		regexStr := genDirectoriesRegex("\\.git", "\\.hg", "\\.svn")
		re, err := regexp.Compile(regexStr)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, re.MatchString(".git"), should.BeTrue)
		assert.Loosely(t, re.MatchString(".git/"), should.BeTrue)
		assert.Loosely(t, re.MatchString("/.git/"), should.BeTrue)
		assert.Loosely(t, re.MatchString("/.hg"), should.BeTrue)
		assert.Loosely(t, re.MatchString("foo/.svn"), should.BeTrue)
		assert.Loosely(t, re.MatchString(`.hg\`), should.BeTrue)
		assert.Loosely(t, re.MatchString(`foo\.svn`), should.BeTrue)

		assert.Loosely(t, re.MatchString(".get"), should.BeFalse)
		assert.Loosely(t, re.MatchString("foo.git"), should.BeFalse)
		assert.Loosely(t, re.MatchString(".svnnnn"), should.BeFalse)
	})
}

func TestProcessIsolateFile(t *testing.T) {
	t.Parallel()

	ftt.Run(`Directory deps should end with osPathSeparator`, t, func(t *ftt.Test) {
		tmpDir := t.TempDir()
		baseDir := filepath.Join(tmpDir, "baseDir")
		secondDir := filepath.Join(tmpDir, "secondDir")
		assert.Loosely(t, os.Mkdir(baseDir, 0700), should.BeNil)
		assert.Loosely(t, os.Mkdir(secondDir, 0700), should.BeNil)
		assert.Loosely(t, os.WriteFile(filepath.Join(baseDir, "foo"), []byte("foo"), 0600), should.BeNil)
		// Note that for "secondDir", its separator is omitted intentionally.
		isolate := `{
			'variables': {
				'files': [
					'../baseDir/',
					'../secondDir',
					'../baseDir/foo',
				],
			},
		}`

		outDir := filepath.Join(tmpDir, "out")
		assert.Loosely(t, os.Mkdir(outDir, 0700), should.BeNil)
		isolatePath := filepath.Join(outDir, "my.isolate")
		assert.Loosely(t, os.WriteFile(isolatePath, []byte(isolate), 0600), should.BeNil)

		opts := &ArchiveOptions{
			Isolate: isolatePath,
		}

		deps, _, err := ProcessIsolate(opts)
		assert.Loosely(t, err, should.BeNil)
		for _, dep := range deps {
			isDir, err := filesystem.IsDir(dep)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, strings.HasSuffix(dep, osPathSeparator), should.Equal(isDir))
		}
	})

	ftt.Run(`Allow missing files and dirs`, t, func(t *ftt.Test) {
		tmpDir := t.TempDir()
		dir1 := filepath.Join(tmpDir, "dir1")
		assert.Loosely(t, os.Mkdir(dir1, 0700), should.BeNil)
		dir2 := filepath.Join(tmpDir, "dir2")
		assert.Loosely(t, os.Mkdir(dir2, 0700), should.BeNil)
		assert.Loosely(t, os.WriteFile(filepath.Join(dir2, "foo"), []byte("foo"), 0600), should.BeNil)
		isolate := `{
			'variables': {
				'files': [
					'../dir1/',
					'../dir2/foo',
					'../nodir1/',
					'../nodir2/nofile',
				],
			},
		}`

		outDir := filepath.Join(tmpDir, "out")
		assert.Loosely(t, os.Mkdir(outDir, 0700), should.BeNil)
		isolatePath := filepath.Join(outDir, "my.isolate")
		assert.Loosely(t, os.WriteFile(isolatePath, []byte(isolate), 0600), should.BeNil)

		opts := &ArchiveOptions{
			Isolate:             isolatePath,
			AllowMissingFileDir: false,
		}

		_, _, err := ProcessIsolate(opts)
		assert.Loosely(t, err, should.NotBeNil)

		opts = &ArchiveOptions{
			Isolate:             isolatePath,
			AllowMissingFileDir: true,
		}

		deps, _, err := ProcessIsolate(opts)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, deps, should.Match([]string{dir1 + osPathSeparator, filepath.Join(dir2, "foo")}))
	})
}
