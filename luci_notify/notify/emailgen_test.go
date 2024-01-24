// Copyright 2018 The LUCI Authors.
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

package notify

import (
	"context"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/config"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEmailGen(t *testing.T) {
	t.Parallel()

	Convey(`bundle`, t, func() {
		c := memory.Use(context.Background())
		c = common.SetAppIDForTest(c, "luci-config")
		c = caching.WithEmptyProcessCache(c)

		chromium := &config.Project{Name: "chromium", Revision: "deadbeef"}
		chromiumKey := datastore.KeyForObj(c, chromium)
		So(datastore.Put(c, chromium), ShouldBeNil)

		templates := []*config.EmailTemplate{
			{
				ProjectKey:          chromiumKey,
				Name:                "default",
				SubjectTextTemplate: "Build {{.Build.Id}} completed",
				BodyHTMLTemplate:    `Build {{.Build.Id}} completed with status {{.Build.Status}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "using_other_files",
				SubjectTextTemplate: "",
				BodyHTMLTemplate: `
Reusing templates from another files.
{{template "inlineEntireFile" .}}
{{template "steps" .}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "inlineEntireFile",
				SubjectTextTemplate: "this file is shared",
				BodyHTMLTemplate:    `Build {{.Build.Id}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "shared",
				SubjectTextTemplate: "this file is shared",
				BodyHTMLTemplate:    `{{define "steps"}}steps of build {{.Build.Id}} go here{{end}}`,
			},
			{
				ProjectKey:          chromiumKey,
				Name:                "bad",
				SubjectTextTemplate: "bad template",
				BodyHTMLTemplate:    `{{.FieldDoesNotExist}}`,
			},
		}
		So(datastore.Put(c, templates), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		bundle, err := getBundle(c, chromium.Name)
		So(err, ShouldBeNil)

		Convey("bundles are cached", func() {
			secondBundle, err := getBundle(c, chromium.Name)
			So(err, ShouldBeNil)
			So(secondBundle, ShouldEqual, bundle) // pointers match
		})

		Convey("caching honors revision", func() {
			chromium.Revision = "badcoffee"
			So(datastore.Put(c, chromium), ShouldBeNil)
			secondBundle, err := getBundle(c, chromium.Name)
			So(err, ShouldBeNil)
			So(secondBundle, ShouldNotEqual, bundle) // pointers mismatch, new bundle
		})

		Convey(`GenerateEmail`, func() {
			input := &notifypb.TemplateInput{
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
			})
		})
	})
}
