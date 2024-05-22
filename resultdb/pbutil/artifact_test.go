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
	"strings"
	"testing"

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
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/artifacts/a%2Fb")
				So(err, ShouldBeNil)
				So(artifactID, ShouldEqual, "a/b")
			})

			Convey(`With a percent sign`, func() {
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/artifacts/a%25b")
				So(err, ShouldBeNil)
				So(artifactID, ShouldEqual, "a%b")
			})

			Convey(`Success with a long artifact name`, func() {
				artName := strings.Repeat("a%2Fb", 100) // 500 characters
				wantArtID := strings.Repeat("a/b", 100)
				_, _, _, gotArtID, err := ParseArtifactName("invocations/inv/artifacts/" + artName)
				So(err, ShouldBeNil)
				So(gotArtID, ShouldEqual, wantArtID)
			})

			Convey(`Failure with a long artifact name over the character limit`, func() {
				artName := strings.Repeat("a", 600)
				_, _, _, _, err := ParseArtifactName("invocations/inv/artifacts/" + artName)
				So(err, ShouldNotBeNil)
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
				_, _, _, artifactID, err := ParseArtifactName("invocations/inv/tests/t/results/r/artifacts/a%2Fb")
				So(err, ShouldBeNil)
				So(artifactID, ShouldEqual, "a/b")
			})
		})
	})
}

func TestArtifactName(t *testing.T) {
	t.Parallel()
	Convey(`ArtifactName`, t, func() {
		Convey(`Invocation level`, func() {
			Convey(`Success`, func() {
				name := InvocationArtifactName("inv", "a")
				So(name, ShouldEqual, "invocations/inv/artifacts/a")
			})
			Convey(`With a slash`, func() {
				name := InvocationArtifactName("inv", "a/b")
				So(name, ShouldEqual, "invocations/inv/artifacts/a%2Fb")
			})
		})

		Convey(`Test result level`, func() {
			Convey(`Success`, func() {
				name := TestResultArtifactName("inv", "t r", "r", "a")
				So(name, ShouldEqual, "invocations/inv/tests/t%20r/results/r/artifacts/a")
			})
			Convey(`With a slash`, func() {
				name := TestResultArtifactName("inv", "t r", "r", "a/b")
				So(name, ShouldEqual, "invocations/inv/tests/t%20r/results/r/artifacts/a%2Fb")
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

func TestArtifactId(t *testing.T) {
	t.Parallel()
	Convey(`ValidateArtifactID`, t, func() {
		Convey(`ASCII printable`, func() {
			err := ValidateArtifactID("ascii.txt")
			So(err, ShouldBeNil)
		})
		Convey(`Unicode printable`, func() {
			err := ValidateArtifactID("unicode ©.txt")
			So(err, ShouldBeNil)
		})
		Convey(`Unprintable`, func() {
			err := ValidateArtifactID("unprintable \a.txt")
			So(err, ShouldErrLike, "does not match")
		})
		Convey(`Starts with dot`, func() {
			err := ValidateArtifactID(".arc.log")
			So(err, ShouldBeNil)
		})

	})
}

func TestIsTextArtifact(t *testing.T) {
	t.Parallel()
	Convey("IsTextArtifact", t, func() {
		Convey("empty content type", func() {
			So(IsTextArtifact(""), ShouldBeFalse)
		})
		Convey("text artifact", func() {
			So(IsTextArtifact("text/plain"), ShouldBeTrue)
		})
		Convey("non text artifact", func() {
			So(IsTextArtifact("image/png"), ShouldBeFalse)
		})
	})
}
