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

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/common/system/filesystem"
)

func init() {
	log.SetOutput(io.Discard)
}

func TestReplaceVars(t *testing.T) {
	t.Parallel()
	Convey(`Variables replacement should be supported in isolate files.`, t, func() {

		opts := &ArchiveOptions{PathVariables: map[string]string{"VAR": "wonderful"}}

		// Single replacement.
		r, err := ReplaceVariables("hello <(VAR) world", opts)
		So(err, ShouldBeNil)
		So(r, ShouldResemble, "hello wonderful world")

		// Multiple replacement.
		r, err = ReplaceVariables("hello <(VAR) <(VAR) world", opts)
		So(err, ShouldBeNil)
		So(r, ShouldResemble, "hello wonderful wonderful world")

		// Replacement of missing variable.
		r, err = ReplaceVariables("hello <(MISSING) world", opts)
		So(err.Error(), ShouldResemble, "no value for variable 'MISSING'")
	})
}

func TestIgnoredPathsRegexp(t *testing.T) {
	t.Parallel()

	Convey(`Ignored file extensions`, t, func() {
		regexStr := genExtensionsRegex("pyc", "swp")
		re, err := regexp.Compile(regexStr)
		So(err, ShouldBeNil)
		So(re.MatchString("a.pyc"), ShouldBeTrue)
		So(re.MatchString("foo/a.pyc"), ShouldBeTrue)
		So(re.MatchString("/b.swp"), ShouldBeTrue)
		So(re.MatchString(`foo\b.swp`), ShouldBeTrue)

		So(re.MatchString("a.py"), ShouldBeFalse)
		So(re.MatchString("b.swppp"), ShouldBeFalse)
	})

	Convey(`Ignored directories`, t, func() {
		regexStr := genDirectoriesRegex("\\.git", "\\.hg", "\\.svn")
		re, err := regexp.Compile(regexStr)
		So(err, ShouldBeNil)
		So(re.MatchString(".git"), ShouldBeTrue)
		So(re.MatchString(".git/"), ShouldBeTrue)
		So(re.MatchString("/.git/"), ShouldBeTrue)
		So(re.MatchString("/.hg"), ShouldBeTrue)
		So(re.MatchString("foo/.svn"), ShouldBeTrue)
		So(re.MatchString(`.hg\`), ShouldBeTrue)
		So(re.MatchString(`foo\.svn`), ShouldBeTrue)

		So(re.MatchString(".get"), ShouldBeFalse)
		So(re.MatchString("foo.git"), ShouldBeFalse)
		So(re.MatchString(".svnnnn"), ShouldBeFalse)
	})
}

func TestProcessIsolateFile(t *testing.T) {
	t.Parallel()

	Convey(`Directory deps should end with osPathSeparator`, t, func() {
		tmpDir := t.TempDir()
		baseDir := filepath.Join(tmpDir, "baseDir")
		secondDir := filepath.Join(tmpDir, "secondDir")
		So(os.Mkdir(baseDir, 0700), ShouldBeNil)
		So(os.Mkdir(secondDir, 0700), ShouldBeNil)
		So(os.WriteFile(filepath.Join(baseDir, "foo"), []byte("foo"), 0600), ShouldBeNil)
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
		So(os.Mkdir(outDir, 0700), ShouldBeNil)
		isolatePath := filepath.Join(outDir, "my.isolate")
		So(os.WriteFile(isolatePath, []byte(isolate), 0600), ShouldBeNil)

		opts := &ArchiveOptions{
			Isolate: isolatePath,
		}

		deps, _, err := ProcessIsolate(opts)
		So(err, ShouldBeNil)
		for _, dep := range deps {
			isDir, err := filesystem.IsDir(dep)
			So(err, ShouldBeNil)
			So(strings.HasSuffix(dep, osPathSeparator), ShouldEqual, isDir)
		}
	})

	Convey(`Allow missing files and dirs`, t, func() {
		tmpDir := t.TempDir()
		dir1 := filepath.Join(tmpDir, "dir1")
		So(os.Mkdir(dir1, 0700), ShouldBeNil)
		dir2 := filepath.Join(tmpDir, "dir2")
		So(os.Mkdir(dir2, 0700), ShouldBeNil)
		So(os.WriteFile(filepath.Join(dir2, "foo"), []byte("foo"), 0600), ShouldBeNil)
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
		So(os.Mkdir(outDir, 0700), ShouldBeNil)
		isolatePath := filepath.Join(outDir, "my.isolate")
		So(os.WriteFile(isolatePath, []byte(isolate), 0600), ShouldBeNil)

		opts := &ArchiveOptions{
			Isolate:             isolatePath,
			AllowMissingFileDir: false,
		}

		_, _, err := ProcessIsolate(opts)
		So(err, ShouldNotBeNil)

		opts = &ArchiveOptions{
			Isolate:             isolatePath,
			AllowMissingFileDir: true,
		}

		deps, _, err := ProcessIsolate(opts)
		So(err, ShouldBeNil)
		So(deps, ShouldResemble, []string{dir1 + osPathSeparator, filepath.Join(dir2, "foo")})
	})
}
