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

package mailtmpl

import (
	"testing"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/luci_notify/api/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBundle(t *testing.T) {
	t.Parallel()

	Convey(`bundle`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-config")

		templates := []*Template{
			{
				Name:                "default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed",
				BodyHTMLTemplate:    `Build {{.Build.Id}} completed with status {{.Build.Status}}`,
			},
			{
				Name:                "using_other_files",
				SubjectTextTemplate: "",
				BodyHTMLTemplate: `
Reusing templates from another files.
{{template "inlineEntireFile" .}}
{{template "steps" .}}`,
			},
			{
				Name:                "inlineEntireFile",
				SubjectTextTemplate: "this file is shared",
				BodyHTMLTemplate:    `Build {{.Build.Id}}`,
			},
			{
				Name:                "shared",
				SubjectTextTemplate: "this file is shared",
				BodyHTMLTemplate:    `{{define "steps"}}steps of build {{.Build.Id}} go here{{end}}`,
			},
			{
				Name:                "bad",
				SubjectTextTemplate: "bad template",
				BodyHTMLTemplate:    `{{.FieldDoesNotExist}}`,
			},
		}
		So(datastore.Put(c, templates), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		bundle := NewBundle(templates)
		So(bundle.Err, ShouldBeNil)
		So(bundle.bodies.Lookup("default"), ShouldNotBeNil)

		Convey(`GenerateEmail`, func() {
			input := &config.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build: &buildbucketpb.Build{
					Id: 54,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "linux-rel",
					},
					Status: buildbucketpb.Status_SUCCESS,
				},
			}
			Convey("simple template", func() {
				subject, body := bundle.GenerateEmail("default", input)
				So(subject, ShouldEqual, "Build 54 completed")
				So(body, ShouldEqual, "Build 54 completed with status SUCCESS")
			})

			Convey("template using other files", func() {
				_, body := bundle.GenerateEmail("using_other_files", input)
				So(body, ShouldEqual, `
Reusing templates from another files.
Build 54
steps of build 54 go here`)
			})

			Convey("error", func() {
				_, body := bundle.GenerateEmail("bad", input)
				So(body, ShouldContainSubstring, "spartan")
				So(body, ShouldContainSubstring, "buildbucket.example.com")
			})
		})

		Convey(`generateDefaultStatusMessage`, func() {
			input := &config.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build: &buildbucketpb.Build{
					Id: 123,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "linux-rel",
					},
					Input: &buildbucketpb.Build_Input{
						GitilesCommit: &buildbucketpb.GitilesCommit{
							Id: "deadbeefdeadbeef",
						},
					},
				},
				MatchingFailedSteps: []*buildbucketpb.Step{
					{Name: "test1"},
					{Name: "test2"},
				},
			}

			So(generateDefaultStatusMessage(input), ShouldEqual,
				`"test1", "test2" on https://buildbucket.example.com/build/123 linux-rel from deadbeefdeadbeef`)
		})

		// Regression test for https://crbug.com/1084358.
		Convey(`GenerateStatusMessage, nil commit`, func() {
			input := &config.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build: &buildbucketpb.Build{
					Id: 123,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "linux-rel",
					},
					Input: &buildbucketpb.Build_Input{GitilesCommit: nil},
				},
				MatchingFailedSteps: []*buildbucketpb.Step{
					{Name: "test1"},
					{Name: "test2"},
				},
			}

			So(generateDefaultStatusMessage(input), ShouldEqual,
				`"test1", "test2" on https://buildbucket.example.com/build/123 linux-rel`)
		})
	})
}

func TestSplitTemplateFile(t *testing.T) {
	t.Parallel()
	Convey(`SplitTemplateFile`, t, func() {
		Convey(`valid template`, func() {
			s, b, err := SplitTemplateFile(`subject

        body`)

			So(err, ShouldBeNil)
			So(s, ShouldEqual, "subject")
			So(b, ShouldEqual, "body")
		})

		Convey(`empty`, func() {
			_, _, err := SplitTemplateFile(``)
			So(err, ShouldErrLike, "empty")
		})

		Convey(`single line`, func() {
			s, b, err := SplitTemplateFile("one line")

			So(err, ShouldBeNil)
			So(s, ShouldEqual, "one line")
			So(b, ShouldEqual, "")
		})

		Convey(`blank second line`, func() {
			s, b, err := SplitTemplateFile(`subject
`)

			So(err, ShouldBeNil)
			So(s, ShouldEqual, "subject")
			So(b, ShouldEqual, "")
		})

		Convey(`non-blank second line`, func() {
			_, _, err := SplitTemplateFile(`subject
        body`)

			So(err, ShouldErrLike, "second line is not blank")
		})

		Convey(`no blank line`, func() {
			_, _, err := SplitTemplateFile(`subject
        body
        second line
        `)
			So(err, ShouldErrLike, "second line is not blank")
		})
	})
}
