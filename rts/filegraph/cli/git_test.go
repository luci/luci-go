// Copyright 2020 The LUCI Authors.
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

package cli

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGit(t *testing.T) {
	t.Parallel()

	// https: //logs.chromium.org/logs/infra/buildbucket/cr-buildbucket.appspot.com/8864634177878601952/+/u/go_test/stdout
	t.Skipf("this test is failing in a weird way; skip for now")

	if testing.Short() {
		t.Skip("Skipping because it is not a short test")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not found: %s", err)
	}

	Convey(`Git`, t, func() {
		tmpd, err := ioutil.TempDir("", "filegraph_git")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmpd)

		git := func(context string) func(args ...string) string {
			return func(args ...string) string {
				out, err := execGit(context)(args...)
				So(err, ShouldBeNil)
				return out
			}
		}

		git(tmpd)("init")

		fooPath := filepath.Join(tmpd, "foo")
		err = ioutil.WriteFile(fooPath, []byte("hello"), 0777)
		So(err, ShouldBeNil)

		// Run in fooBar context.
		git(fooPath)("add", fooPath)
		git(tmpd)("commit", "-a", "-m", "message")

		out := git(fooPath)("status")
		So(out, ShouldContainSubstring, "working tree clean")

		repoDir, err := ensureSameRepo(tmpd, fooPath)
		So(err, ShouldBeNil)
		So(repoDir, ShouldEqual, tmpd)
	})
}
