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

package version

import (
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRecoverSymlinkPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	Convey("recoverSymlinkPath works", t, func(c C) {
		So(recoverSymlinkPath("/a/b/.cipd/pkgs/cipd_mac-amd64_L8/08c6146/cipd"), ShouldEqual, "/a/b/cipd")
		So(recoverSymlinkPath("/a/b/.cipd/pkgs/cipd_mac-amd64_L8/08c6146/c/d/cipd"), ShouldEqual, "/a/b/c/d/cipd")
		So(recoverSymlinkPath(".cipd/pkgs/cipd_mac-amd64_L8/08c6146/a"), ShouldEqual, "a")
	})

	Convey("recoverSymlinkPath handles bad paths", t, func(c C) {
		So(recoverSymlinkPath(""), ShouldEqual, "")
		So(recoverSymlinkPath("/a/b/c"), ShouldEqual, "")
		So(recoverSymlinkPath("/a/b/c/d/e/f"), ShouldEqual, "")
		So(recoverSymlinkPath("/a/b/c/.cipd/pkgs/d"), ShouldEqual, "")
		So(recoverSymlinkPath("/a/b/c/.cipd/pkgs/abc/d"), ShouldEqual, "")
	})
}
