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
	"context"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
)

func TestBundle(t *testing.T) {
	t.Parallel()

	ftt.Run(`bundle`, t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-config")

		templates := []*Template{
			{
				Name:                "default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed",
				BodyHTMLTemplate:    `Build {{.Build.Id}} completed with status {{.Build.Status}}`,
			},
			{
				Name:                "markdown",
				SubjectTextTemplate: "Build {{.Build.Id}}",
				BodyHTMLTemplate:    `{{.Build.SummaryMarkdown | markdown}}`,
			},
			{
				Name:                "using_other_files",
				SubjectTextTemplate: "",
				BodyHTMLTemplate: `
Reusing templates from other files.
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
		assert.Loosely(t, datastore.Put(c, templates), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		bundle := NewBundle(templates)
		assert.Loosely(t, bundle.Err, should.BeNil)
		assert.Loosely(t, bundle.bodies.Lookup("default"), should.NotBeNil)

		t.Run(`GenerateEmail`, func(t *ftt.Test) {
			input := &config.TemplateInput{
				BuildbucketHostname: "buildbucket.example.com",
				Build: &buildbucketpb.Build{
					Id: 54,
					Builder: &buildbucketpb.BuilderID{
						Project: "chromium",
						Bucket:  "ci",
						Builder: "linux-rel",
					},
					Status:          buildbucketpb.Status_SUCCESS,
					SummaryMarkdown: "*ninja* compiled `11` files",
				},
			}

			t.Run("simple template", func(t *ftt.Test) {
				subject, body := bundle.GenerateEmail("default", input)
				// Assert on body first, since errors would be rendered in body.
				assert.Loosely(t, body, should.Equal("Build 54 completed with status SUCCESS"))
				assert.Loosely(t, subject, should.Equal("Build 54 completed"))
			})

			t.Run("markdown", func(t *ftt.Test) {
				_, body := bundle.GenerateEmail("markdown", input)
				assert.Loosely(t, body, should.Equal("<p><em>ninja</em> compiled <code>11</code> files</p>\n"))
			})

			t.Run("template using other files", func(t *ftt.Test) {
				_, body := bundle.GenerateEmail("using_other_files", input)
				assert.Loosely(t, body, should.Equal(`
Reusing templates from other files.
Build 54
steps of build 54 go here`))
			})

			t.Run("error", func(t *ftt.Test) {
				_, body := bundle.GenerateEmail("bad", input)
				assert.Loosely(t, body, should.ContainSubstring("spartan"))
				assert.Loosely(t, body, should.ContainSubstring("buildbucket.example.com"))
			})
		})

		t.Run(`generateDefaultStatusMessage`, func(t *ftt.Test) {
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

			assert.Loosely(t, generateDefaultStatusMessage(input), should.Equal(
				`"test1", "test2" on https://buildbucket.example.com/build/123 linux-rel from deadbeefdeadbeef`))
		})

		// Regression test for https://crbug.com/1084358.
		t.Run(`GenerateStatusMessage, nil commit`, func(t *ftt.Test) {
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

			assert.Loosely(t, generateDefaultStatusMessage(input), should.Equal(
				`"test1", "test2" on https://buildbucket.example.com/build/123 linux-rel`))
		})
	})
}

func TestSplitTemplateFile(t *testing.T) {
	t.Parallel()
	ftt.Run(`SplitTemplateFile`, t, func(t *ftt.Test) {
		t.Run(`valid template`, func(t *ftt.Test) {
			s, b, err := SplitTemplateFile(`subject

        body`)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Equal("subject"))
			assert.Loosely(t, b, should.Equal("body"))
		})

		t.Run(`empty`, func(t *ftt.Test) {
			_, _, err := SplitTemplateFile(``)
			assert.Loosely(t, err, should.ErrLike("empty"))
		})

		t.Run(`single line`, func(t *ftt.Test) {
			s, b, err := SplitTemplateFile("one line")

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Equal("one line"))
			assert.Loosely(t, b, should.BeEmpty)
		})

		t.Run(`blank second line`, func(t *ftt.Test) {
			s, b, err := SplitTemplateFile(`subject
`)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Equal("subject"))
			assert.Loosely(t, b, should.BeEmpty)
		})

		t.Run(`non-blank second line`, func(t *ftt.Test) {
			_, _, err := SplitTemplateFile(`subject
        body`)

			assert.Loosely(t, err, should.ErrLike("second line is not blank"))
		})

		t.Run(`no blank line`, func(t *ftt.Test) {
			_, _, err := SplitTemplateFile(`subject
        body
        second line
        `)
			assert.Loosely(t, err, should.ErrLike("second line is not blank"))
		})
	})
}
