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
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/clock/testclock"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// validArtifacts returns two valid Artifact samples.
func validArtifacts(now time.Time) (*pb.Artifact, *pb.Artifact) {
	et, _ := ptypes.TimestampProto(now.Add(24 * time.Hour))
	art1 := &pb.Artifact{
		Name:               "this is artifact 1",
		FetchUrl:           "https://foo/bar",
		FetchUrlExpiration: et,
		ContentType:        "text/plain",
		SizeBytes:          1024,
	}
	art2 := &pb.Artifact{
		Name:               "this is artifact 2",
		FetchUrl:           "https://foo/bar/log.png",
		FetchUrlExpiration: et,
		ContentType:        "image/png",
		SizeBytes:          1024,
	}
	return art1, art2
}

// invalidArtifacts returns two invalid Artifact samples.
func invalidArtifacts(now time.Time) (*pb.Artifact, *pb.Artifact) {
	et, _ := ptypes.TimestampProto(now.Add(24 * time.Hour))
	art1 := &pb.Artifact{
		Name:               " this is a bad artifact name.",
		FetchUrl:           "https://foo/bar",
		FetchUrlExpiration: et,
		ContentType:        "text/plain",
		SizeBytes:          1024,
	}
	art2 := &pb.Artifact{
		Name:               "this has a bad fetch url",
		FetchUrl:           "isolate://foo/bar/log.png",
		FetchUrlExpiration: et,
		ContentType:        "image/png",
		SizeBytes:          1024,
	}
	return art1, art2
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

func TestValidateArtifact(t *testing.T) {
	t.Parallel()
	now := testclock.TestRecentTimeUTC
	validate := func(art *pb.Artifact) error {
		return ValidateArtifact(art)
	}

	Convey("Succeeds", t, func() {
		art, _ := validArtifacts(now)
		So(validate(art), ShouldBeNil)

		Convey("with no FetchUrlExpiration", func() {
			art.FetchUrlExpiration = nil
			So(validate(art), ShouldBeNil)
		})

		Convey("with empty ContentType", func() {
			art.ContentType = ""
			So(validate(art), ShouldBeNil)
		})

		Convey("with 0 in Size", func() {
			art.SizeBytes = 0
			So(validate(art), ShouldBeNil)
		})
	})

	Convey("Fails", t, func() {
		art, _ := validArtifacts(now)

		Convey("with nil", func() {
			So(validate(nil), ShouldErrLike, unspecified())
		})

		Convey("with empty Name", func() {
			art.Name = ""
			So(validate(art), ShouldErrLike, fieldUnspecified("name"))
		})

		Convey("with invalid Name", func() {
			badInputs := []string{
				" name", "name ", "name ##", "name ?", "name 1@",
				strings.Repeat("n", 256+1),
			}
			for _, in := range badInputs {
				art.Name = in
				So(validate(art), ShouldErrLike, fieldDoesNotMatch("name", artifactNameRe))
			}
		})

		Convey("with empty FetchUrl", func() {
			art.FetchUrl = ""
			So(validate(art), ShouldErrLike, "empty url")
		})

		Convey("with invalid URI", func() {
			art.FetchUrl = "a/b"
			So(validate(art), ShouldErrLike, "invalid URI for request")
		})

		Convey("with unsupported Scheme", func() {
			badInputs := []string{
				"/my_test", "http://host/page", "isolate://a/b",
			}
			for _, in := range badInputs {
				art.FetchUrl = in
				So(validate(art), ShouldErrLike, "the URL scheme is not HTTPS")
			}
		})

		Convey("without host", func() {
			art.FetchUrl = "https://"
			So(validate(art), ShouldErrLike, "missing host")
		})
	})
}
