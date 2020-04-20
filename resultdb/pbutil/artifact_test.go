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

package pbutil

import (
	"testing"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseArtifactName(t *testing.T) {
	t.Parallel()
	Convey(`ParseArtifactName`, t, func() {
		Convey(`Invocation level`, func() {
			Convey(`Success`, func() {
				invocationID, testID, resultID, artifactID, err := ParseArtifactName("invocations/inv/artifacts/a")
				So(err, ShouldBeNil)
				So(invocationID, ShouldEqual, "inv")
				So(testID, ShouldEqual, "")
				So(resultID, ShouldEqual, "")
				So(artifactID, ShouldEqual, "a")
			})

			Convey(`With a slash`, func() {
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/artifacts/a/b")
				So(err, ShouldBeNil)
				So(artifactID, ShouldEqual, "a/b")
			})
		})

		Convey(`Test result level`, func() {
			Convey(`Success`, func() {
				invocationID, testID, resultID, artifactID, err := ParseArtifactName("invocations/inv/tests/t/results/r/artifacts/a")
				So(err, ShouldBeNil)
				So(invocationID, ShouldEqual, "inv")
				So(testID, ShouldEqual, "t")
				So(resultID, ShouldEqual, "r")
				So(artifactID, ShouldEqual, "a")
			})

			Convey(`With a slash in test ID`, func() {
				_, testID, _, _, err := ParseArtifactName("invocations/inv/tests/t%2F/results/r/artifacts/a/b")
				So(err, ShouldBeNil)
				So(testID, ShouldEqual, "t/")
			})

			Convey(`With a slash`, func() {
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/tests/t/results/r/artifacts/a/b")
				So(err, ShouldBeNil)
				So(artifactID, ShouldEqual, "a/b")
			})
		})
	})
}

func TestValidateArtifactName(t *testing.T) {
	t.Parallel()
	Convey(`ValidateArtifactName`, t, func() {
		Convey(`Invocation level`, func() {
			err := ValidateArtifactName("invocations/inv/artifacts/a/b")
			So(err, ShouldBeNil)
		})
		Convey(`Test result level`, func() {
			err := ValidateArtifactName("invocations/inv/tests/t/results/r/artifacts/a")
			So(err, ShouldBeNil)
		})
		Convey(`Invalid`, func() {
			err := ValidateArtifactName("abc")
			So(err, ShouldErrLike, "does not match")
		})
	})
}

func TestValidateSinkArtifacts(t *testing.T) {
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
			So(ValidateSinkArtifacts(nil), ShouldBeNil)
		})

		Convey("with valid artifacts", func() {
			So(ValidateSinkArtifacts(validArts), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		expected := "body: either file_path or contents must be provided"

		Convey("with invalid artifacts", func() {
			So(ValidateSinkArtifacts(invalidArts), ShouldErrLike, expected)
		})

		Convey("with a mix of valid and invalid artifacts", func() {
			invalidArts["art2"] = validArts["art2"]
			So(ValidateSinkArtifacts(invalidArts), ShouldErrLike, expected)
		})
	})
}
