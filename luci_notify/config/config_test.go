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
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
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
							bucket: "luci.chromium.ci"
							name: "linux"
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
							bucket: "luci.v8.ci"
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

		var notifiers []*Notifier
		So(datastore.GetAll(c, datastore.NewQuery("Notifier"), &notifiers), ShouldBeNil)
		So(notifiers, ShouldResemble, []*Notifier{
			{
				Parent:     datastore.MakeKey(c, "Project", "chromium"),
				Name:       "chromium-notifier",
				BuilderIDs: []string{"buildbucket/chromium/luci.chromium.ci/linux"},
				Notifications: []NotificationConfig{
					{
						OnSuccess:       true,
						EmailRecipients: []string{"johndoe@chromium.org", "janedoe@chromium.org"},
					},
				},
			},
			{
				Parent:     datastore.MakeKey(c, "Project", "v8"),
				Name:       "v8-notifier",
				BuilderIDs: []string{"buildbucket/v8/luci.v8.ci/win"},
				Notifications: []NotificationConfig{
					{
						OnChange:        true,
						EmailRecipients: []string{"johndoe@v8.org", "janedoe@v8.org"},
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

		Convey("delete project", func() {
			delete(cfg, "projects/v8")
			So(updateProjects(c), ShouldBeNil)

			datastore.GetTestable(c).CatchupIndexes()

			v8 := &Project{Name: "v8"}
			So(datastore.Get(c, v8), ShouldEqual, datastore.ErrNoSuchEntity)
			v8Key := datastore.KeyForObj(c, v8)

			var notifiers []*Notifier
			So(datastore.GetAll(c, datastore.NewQuery("Notifier").Ancestor(v8Key), &notifiers), ShouldBeNil)
			So(notifiers, ShouldBeEmpty)

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
