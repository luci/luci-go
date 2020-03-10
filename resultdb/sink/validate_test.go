// Copyright 2019 The LUCI Authors.
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
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func createFile(s string) string {
	f, err := ioutil.TempFile("", "test_foo")
	So(err, ShouldBeNil)
	defer f.Close()

	if len(s) > 0 {
		l, err := f.WriteString(s)
		So(err, ShouldBeNil)
		So(l, ShouldEqual, len(s))
	}
	return f.Name()
}

func createArtifactWithFP(p, ct string) *sinkpb.Artifact {
	return &sinkpb.Artifact{
		Body: &sinkpb.Artifact_FilePath{
			FilePath: p,
		},
		ContentType: ct,
	}
}

func TestValidate(t *testing.T) {
	Convey("Test", t, func() {
		validFP := createFile(`"this is an artifact."`)
		artifactWithValidFP := createArtifactWithFP(validFP, "text/plain")
		defer os.Remove(validFP)

		Convey("validateUploadTestResult", func() {
			Convey("Success", func() {
				Convey("With artifacts", func() {
					msg := &sinkpb.TestResult{
						TestId:   "test-id-1",
						ResultId: "result-id-a",
						InputArtifacts: map[string]*sinkpb.Artifact{
							"artifact1": artifactWithValidFP,
						},
					}
					So(validateUploadTestResult(msg), ShouldBeNil)
				})

				Convey("Without artifacts", func() {
					msg := &sinkpb.TestResult{
						TestId:   "test-id-1",
						ResultId: "result-id-a",
					}
					So(validateUploadTestResult(msg), ShouldBeNil)
				})
			})

			Convey("Fails", func() {
				Convey("With invalid TestID", func() {
					tid := ""
					em := fmt.Sprintf("TestID %q", tid)
					msg := &sinkpb.TestResult{TestId: tid, ResultId: "resID"}
					So(validateUploadTestResult(msg), ShouldErrLike, em)
				})

				Convey("With invalid ResultID", func() {
					rid := ""
					em := fmt.Sprintf("ResultID %q", rid)
					msg := &sinkpb.TestResult{TestId: "testID", ResultId: rid}
					So(validateUploadTestResult(msg), ShouldErrLike, em)
				})

				Convey("With invalid artifacts", func() {
					an := " this is an invalid name ----"
					msg := &sinkpb.TestResult{
						TestId:   "test-id-1",
						ResultId: "result-id-a",
						InputArtifacts: map[string]*sinkpb.Artifact{
							an: artifactWithValidFP,
						},
					}
					em := fmt.Sprintf("Artifact.name %q", an)
					So(validateUploadTestResult(msg), ShouldErrLike, em)
				})
			})
		})

		Convey("validateUploadTestResultFile", func() {
			Convey("Success", func() {
				Convey("With a valid file path", func() {
					msg := &sinkpb.TestResultFile{Path: validFP}
					So(validateUploadTestResultFile(msg), ShouldBeNil)
				})
			})

			Convey("Fails", func() {
				Convey("With an empty file path", func() {
					msg := &sinkpb.TestResultFile{}
					So(validateUploadTestResultFile(msg), ShouldNotBeNil)
				})
				Convey("With an non-existing file path", func() {
					p := "this doesn nnnnnot exisssst"
					_, err := os.Stat(p)
					So(err, ShouldNotBeNil)
					msg := &sinkpb.TestResultFile{Path: p}
					So(validateUploadTestResultFile(msg), ShouldNotBeNil)
				})
			})
		})

		Convey("validateArtifact", func() {
			Convey("Successes", func() {
				So(validateArtifact("name", artifactWithValidFP), ShouldBeNil)
			})

			Convey("Fails", func() {
				Convey("With a bad name", func() {
					n := " bad artifact name-"
					err := validateArtifact(n, artifactWithValidFP)
					So(err, ShouldErrLike, fmt.Sprintf("Artifact.name %q", n))
				})

				Convey("With a non-existing file", func() {
					p := "this doooesn nnnnnot exisssssst"
					_, err := os.Stat(p)
					So(err, ShouldNotBeNil)
					msg := createArtifactWithFP(p, "text/plain")
					So(validateArtifact("name", msg), ShouldNotBeNil)
				})
			})
		})

		Convey("checkFileAccess", func() {
			Convey("Success", func() {
				Convey("With a valid file", func() {
					So(checkFileAccess(validFP), ShouldBeNil)
				})
				Convey("With an empty file", func() {
					p := createFile("")
					defer os.Remove(p)
					So(checkFileAccess(p), ShouldBeNil)
				})
			})

			Convey("Fails", func() {
				Convey("With a non-existing file", func() {
					p := "this fie doesnt existttt"
					_, err := os.Stat(p)
					So(err, ShouldNotBeNil)
					So(checkFileAccess(p), ShouldNotBeNil)
				})
				Convey("With an existing directory", func() {
					p, err := ioutil.TempDir("", "test_foo")
					So(err, ShouldBeNil)
					defer os.RemoveAll(p)
					So(checkFileAccess(p), ShouldErrLike, "not a regular file")
				})
			})
		})
	})
}
