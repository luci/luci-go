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

package gs

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidatePath(t *testing.T) {
	t.Parallel()

	Convey("ValidatePath works", t, func() {
		good := []string{
			"/z/z",
			"/z/z/z/z/z/z/z",
		}
		for _, p := range good {
			So(ValidatePath(p), ShouldBeNil)
		}
		bad := []string{
			"",
			"z/z/z",
			"/zzz",
			"//zz",
			"/z//z",
			"/z/z/",
			"/z/z\n",
			"/z/🤦",
		}
		for _, p := range bad {
			So(ValidatePath(p), ShouldNotBeNil)
		}
	})
}

func TestSplitPath(t *testing.T) {
	t.Parallel()

	Convey("SplitPath works", t, func() {
		bucket, path := SplitPath("/a/b/c/d")
		So(bucket, ShouldEqual, "a")
		So(path, ShouldEqual, "b/c/d")

		So(func() { SplitPath("") }, ShouldPanic)
		So(func() { SplitPath("/zzz") }, ShouldPanic)
	})
}
