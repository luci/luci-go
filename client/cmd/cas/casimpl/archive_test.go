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

package casimpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRoot(t *testing.T) {
	t.Parallel()

	Convey(`Basic`, t, func() {
		dirs := make(scatterGather)
		So(dirs.Add("wd1", "rel"), ShouldBeNil)
		So(dirs.Add("wd1", "rel2"), ShouldBeNil)

		wd, err := getRoot(dirs)
		So(err, ShouldBeNil)
		So(wd, ShouldEqual, "wd1")

		So(dirs.Add("wd2", "rel3"), ShouldBeNil)

		_, err = getRoot(dirs)
		So(err, ShouldNotBeNil)
	})
}

func TestLoadPathsJSON(t *testing.T) {
	t.Parallel()

	Convey(`Basic`, t, func() {
		dir := t.TempDir()
		input := [][2]string{
			{dir, "foo.txt"},
			{dir, "bar.txt"},
		}
		b, err := json.Marshal(input)
		So(err, ShouldBeNil)

		pathsJSON := filepath.Join(dir, "paths.json")
		err = os.WriteFile(pathsJSON, b, 0o600)
		So(err, ShouldBeNil)

		res, err := loadPathsJSON(pathsJSON)
		So(err, ShouldBeNil)

		So(res, ShouldResemble, scatterGather{
			"foo.txt": dir,
			"bar.txt": dir,
		})
	})
}
