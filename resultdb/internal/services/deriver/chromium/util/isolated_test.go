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

package util

import (
	"testing"

	"go.chromium.org/luci/common/isolated"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestIsolatedUtils(t *testing.T) {
	t.Parallel()

	Convey(`Conversion to *pb.Artifact works`, t, func() {
		f := &isolated.File{Digest: "400dc0ffee"}
		expectedArt := &pb.Artifact{
			FetchUrl: "isolate://iso.appspot.com/default-zip/400dc0ffee",
		}

		Convey(`with text type`, func() {
			expectedArt.Name = "a/foo.txt"
			expectedArt.ContentType = "text/plain"
			So(IsolatedFileToArtifact("iso.appspot.com", "default-zip", "a/foo.txt", f),
				ShouldResembleProto, expectedArt)
		})

		Convey(`with image type`, func() {
			expectedArt.Name = "a/foo.png"
			expectedArt.ContentType = "image/png"
			So(IsolatedFileToArtifact("iso.appspot.com", "default-zip", "a/foo.png", f),
				ShouldResembleProto, expectedArt)
		})

		Convey(`with file size`, func() {
			f.Size = new(int64)
			*f.Size = 4096
			expectedArt.Name = "."
			expectedArt.Size = 4096
			So(IsolatedFileToArtifact("iso.appspot.com", "default-zip", "", f),
				ShouldResembleProto, expectedArt)
		})

		Convey(`with non-canonical paths`, func() {
			expectedArt.Name = "a/foo.txt"
			expectedArt.ContentType = "text/plain"
			So(IsolatedFileToArtifact("iso.appspot.com", "default-zip", ".\\b\\..\\a\\foo.txt", f),
				ShouldResembleProto, expectedArt)
		})

		Convey(`with backslashes`, func() {
			expectedArt.Name = "a/foo.txt"
			expectedArt.ContentType = "text/plain"
			So(IsolatedFileToArtifact("iso.appspot.com", "default-zip", "a\\foo.txt", f),
				ShouldResembleProto, expectedArt)
		})
	})
}
