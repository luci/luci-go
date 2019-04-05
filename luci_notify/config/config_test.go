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

package config

import (
	"testing"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"

	notifypb "go.chromium.org/luci/luci_notify/api/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigIngestion(t *testing.T) {
	t.Parallel()

	Convey(`updateProjects`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-notify")
		c = gologger.StdConfig.Use(c)
		c = logging.SetLevel(c, logging.Debug)
		cfg := map[config.Set]memory.Files{
			"projects/chromium": {
				"luci-notify.cfg": `
					notifiers {
						name: "chromium-notifier"
						notifications {
							on_success: true
							email {
								recipients: "johndoe@chromium.org"
								recipients: "janedoe@chromium.org"
							}
						}
						builders {
							bucket: "ci"
							name: "linux"
							repository: "https://chromium.googlesource.com/chromium/src"
						}
					}
				`,
				"luci-notify/email-templates/a.template": "a\n\nchromium",
				"luci-notify/email-templates/b.template": "b\n\nchromium",
			},
			"projects/v8": {
				"luci-notify.cfg": `
					notifiers {
						name: "v8-notifier"
						notifications {
							on_change: true
							email {
								recipients: "johndoe@v8.org"
								recipients: "janedoe@v8.org"
							}
						}
						builders {
							bucket: "ci"
							name: "win"
						}
					}
				`,
				"luci-notify/email-templates/a.template": "a\n\nv8",
				"luci-notify/email-templates/b.template": "b\n\nv8",
			},
		}
		c = WithConfigService(c, memory.New(cfg))

		err := updateProjects(c)
		So(err, ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		var projects []*Project
		So(datastore.GetAll(c, datastore.NewQuery("Project"), &projects), ShouldBeNil)
		So(len(projects), ShouldEqual, 2)
		So(projects[0].Name, ShouldEqual, "chromium")
		So(projects[1].Name, ShouldEqual, "v8")

		var builders []*Builder
		So(datastore.GetAll(c, datastore.NewQuery("Builder"), &builders), ShouldBeNil)
		So(builders, ShouldResemble, []*Builder{
			{
				ProjectKey: datastore.MakeKey(c, "Project", "chromium"),
				ID:         "ci/linux",
				Repository: "https://chromium.googlesource.com/chromium/src",
				Notifications: notifypb.Notifications{
					Notifications: []*notifypb.Notification{
						{
							OnSuccess: true,
							Email: &notifypb.Notification_Email{
								Recipients: []string{"johndoe@chromium.org", "janedoe@chromium.org"},
							},
						},
					},
				},
			},
			{
				ProjectKey: datastore.MakeKey(c, "Project", "v8"),
				ID:         "ci/win",
				Notifications: notifypb.Notifications{
					Notifications: []*notifypb.Notification{
						{
							OnChange: true,
							Email: &notifypb.Notification_Email{
								Recipients: []string{"johndoe@v8.org", "janedoe@v8.org"},
							},
						},
					},
				},
			},
		})

		var emailTemplates []*EmailTemplate
		So(datastore.GetAll(c, datastore.NewQuery("EmailTemplate"), &emailTemplates), ShouldBeNil)
		So(emailTemplates, ShouldResemble, []*EmailTemplate{
			{
				ProjectKey:          datastore.MakeKey(c, "Project", "chromium"),
				Name:                "a",
				SubjectTextTemplate: "a",
				BodyHTMLTemplate:    "chromium",
				DefinitionURL:       "https://example.com/view/here/luci-notify/email-templates/a.template",
			},
			{
				ProjectKey:          datastore.MakeKey(c, "Project", "chromium"),
				Name:                "b",
				SubjectTextTemplate: "b",
				BodyHTMLTemplate:    "chromium",
				DefinitionURL:       "https://example.com/view/here/luci-notify/email-templates/b.template",
			},
			{
				ProjectKey:          datastore.MakeKey(c, "Project", "v8"),
				Name:                "a",
				SubjectTextTemplate: "a",
				BodyHTMLTemplate:    "v8",
				DefinitionURL:       "https://example.com/view/here/luci-notify/email-templates/a.template",
			},
			{
				ProjectKey:          datastore.MakeKey(c, "Project", "v8"),
				Name:                "b",
				SubjectTextTemplate: "b",
				BodyHTMLTemplate:    "v8",
				DefinitionURL:       "https://example.com/view/here/luci-notify/email-templates/b.template",
			},
		})

		Convey("preserve updated fields", func() {
			// Update the Chromium builder in the datastore, simulating that some request was handled.
			chromiumBuilder := builders[0]
			chromiumBuilder.Status = buildbucketpb.Status_FAILURE
			chromiumBuilder.Revision = "abc123"
			chromiumBuilder.GitilesCommits = notifypb.GitilesCommits{
				Commits: []*buildbucketpb.GitilesCommit{
					{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Id:      "deadbeefdeadbeefdeadbeef",
					},
				},
			}
			So(datastore.Put(c, chromiumBuilder), ShouldBeNil)

			datastore.GetTestable(c).CatchupIndexes()

			So(updateProjects(c), ShouldBeNil)

			chromium := &Project{Name: "chromium"}
			chromiumKey := datastore.KeyForObj(c, chromium)

			var newBuilders []*Builder
			So(datastore.GetAll(c, datastore.NewQuery("Builder").Ancestor(chromiumKey), &newBuilders), ShouldBeNil)
			So(newBuilders, ShouldHaveLength, 1)
			// Check the fields we care about explicitly, because generated proto structs may have
			// size caches which are updated.
			So(newBuilders[0].Status, ShouldEqual, chromiumBuilder.Status)
			So(newBuilders[0].Revision, ShouldResemble, chromiumBuilder.Revision)
			So(&newBuilders[0].GitilesCommits, ShouldResembleProto, &chromiumBuilder.GitilesCommits)
		})

		Convey("delete project", func() {
			delete(cfg, "projects/v8")
			So(updateProjects(c), ShouldBeNil)

			datastore.GetTestable(c).CatchupIndexes()

			v8 := &Project{Name: "v8"}
			So(datastore.Get(c, v8), ShouldEqual, datastore.ErrNoSuchEntity)
			v8Key := datastore.KeyForObj(c, v8)

			var builders []*Builder
			So(datastore.GetAll(c, datastore.NewQuery("Builder").Ancestor(v8Key), &builders), ShouldBeNil)
			So(builders, ShouldBeEmpty)

			var emailTemplates []*EmailTemplate
			So(datastore.GetAll(c, datastore.NewQuery("EmailTemplate").Ancestor(v8Key), &emailTemplates), ShouldBeNil)
			So(emailTemplates, ShouldBeEmpty)
		})

		Convey("rename email template", func() {
			oldName := "luci-notify/email-templates/a.template"
			newName := "luci-notify/email-templates/c.template"
			chromiumCfg := cfg["projects/chromium"]
			chromiumCfg[newName] = chromiumCfg[oldName]
			delete(chromiumCfg, oldName)

			So(updateProjects(c), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			var emailTemplates []*EmailTemplate
			q := datastore.NewQuery("EmailTemplate").Ancestor(datastore.MakeKey(c, "Project", "chromium"))
			So(datastore.GetAll(c, q, &emailTemplates), ShouldBeNil)
			So(len(emailTemplates), ShouldEqual, 2)
			So(emailTemplates[0].Name, ShouldEqual, "b")
			So(emailTemplates[1].Name, ShouldEqual, "c")
		})
	})
}
