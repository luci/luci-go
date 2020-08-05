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

package sink

import (
	"testing"

	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateArtifacts(t *testing.T) {
	t.Parallel()
	// valid artifacts
	validArts := map[string]*sinkpb.Artifact{
		"art1": {
			Body:        &sinkpb.Artifact_FilePath{"/tmp/foo"},
			ContentType: "text/plain",
		},
		"art2": {
			Body:        &sinkpb.Artifact_Contents{[]byte("contents")},
			ContentType: "text/plain",
		},
	}
	// invalid artifacts
	invalidArts := map[string]*sinkpb.Artifact{
		"art1": {ContentType: "text/plain"},
	}

	Convey("Succeeds", t, func() {
		Convey("with no artifact", func() {
			So(validateArtifacts(nil), ShouldBeNil)
		})

		Convey("with valid artifacts", func() {
			So(validateArtifacts(validArts), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		expected := "body: either file_path or contents must be provided"

		Convey("with invalid artifacts", func() {
			So(validateArtifacts(invalidArts), ShouldErrLike, expected)
		})

		Convey("with a mix of valid and invalid artifacts", func() {
			invalidArts["art2"] = validArts["art2"]
			So(validateArtifacts(invalidArts), ShouldErrLike, expected)
		})
	})
}
