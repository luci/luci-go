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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	mem_ds "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
)

func TestConfigIngestion(t *testing.T) {
	t.Parallel()

	ftt.Run(`updateProjects`, t, func(t *ftt.Test) {
		c := mem_ds.Use(context.Background())
		c = logging.SetLevel(c, logging.Debug)
		c = common.SetAppIDForTest(c, "luci-notify")

		cfg := map[config.Set]memory.Files{
			"projects/chromium": {
				"luci-notify.cfg": `
					notifiers {
						name: "chromium-notifier"
						notifications {
							on_occurrence: SUCCESS
							email {
								recipients: "johndoe@chromium.org"
								recipients: "janedoe@chromium.org"
							}
						}
						tree_closers {
							tree_status_host: "chromium-status.appspot.com"
							failed_step_regexp: "test"
							failed_step_regexp_exclude: "experimental_test"
						}
						builders {
							bucket: "ci"
							name: "linux"
							repository: "https://chromium.googlesource.com/chromium/src"
						}
					}`,
				"luci-notify/email-templates/a.template": "a\n\nchromium",
				"luci-notify/email-templates/b.template": "b\n\nchromium",
			},
			"projects/v8": {
				"luci-notify.cfg": `
					tree_closing_enabled: true
					notifiers {
						name: "v8-notifier"
						notifications {
							on_new_status: SUCCESS
							on_new_status: FAILURE
							on_new_status: INFRA_FAILURE
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
		c = cfgclient.Use(c, memory.New(cfg))

		err := updateProjects(c)
		assert.Loosely(t, err, should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		var projects []*Project
		assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("Project"), &projects), should.BeNil)
		assert.Loosely(t, len(projects), should.Equal(2))
		assert.Loosely(t, projects[0].Name, should.Equal("chromium"))
		assert.Loosely(t, projects[0].TreeClosingEnabled, should.BeFalse)
		assert.Loosely(t, projects[1].Name, should.Equal("v8"))
		assert.Loosely(t, projects[1].TreeClosingEnabled, should.BeTrue)

		var builders []*Builder
		assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("Builder"), &builders), should.BeNil)

		// Can't test 'builders' using ShouldResembleProto, as the base object
		// isn't a proto, it just contains one. Can't test it using
		// ShouldResemble either, as it has a bug where it doesn't terminate
		// for protos. So we have to manually pull out the elements of the
		// array and test the different parts individually.
		assert.Loosely(t, builders, should.HaveLength(2))
		b := builders[0]
		assert.Loosely(t, b.ProjectKey, should.Resemble(datastore.MakeKey(c, "Project", "chromium")))
		assert.Loosely(t, b.ID, should.Equal("ci/linux"))
		assert.Loosely(t, b.Repository, should.Equal("https://chromium.googlesource.com/chromium/src"))
		assert.Loosely(t, &b.Notifications, should.Resemble(&notifypb.Notifications{
			Notifications: []*notifypb.Notification{
				{
					OnOccurrence: []buildbucketpb.Status{
						buildbucketpb.Status_SUCCESS,
					},
					Email: &notifypb.Notification_Email{
						Recipients: []string{"johndoe@chromium.org", "janedoe@chromium.org"},
					},
				},
			},
		}))

		b = builders[1]
		assert.Loosely(t, b.ProjectKey, should.Resemble(datastore.MakeKey(c, "Project", "v8")))
		assert.Loosely(t, b.ID, should.Equal("ci/win"))
		assert.Loosely(t, &b.Notifications, should.Resemble(&notifypb.Notifications{
			Notifications: []*notifypb.Notification{
				{
					OnNewStatus: []buildbucketpb.Status{
						buildbucketpb.Status_SUCCESS,
						buildbucketpb.Status_FAILURE,
						buildbucketpb.Status_INFRA_FAILURE,
					},
					Email: &notifypb.Notification_Email{
						Recipients: []string{"johndoe@v8.org", "janedoe@v8.org"},
					},
				},
			},
		}))

		var emailTemplates []*EmailTemplate
		assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("EmailTemplate"), &emailTemplates), should.BeNil)
		assert.Loosely(t, emailTemplates, should.Resemble([]*EmailTemplate{
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
		}))

		var treeClosers []*TreeCloser
		assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("TreeCloser"), &treeClosers), should.BeNil)

		// As above, can't use ShouldResemble or ShouldResembleProto directly.
		assert.Loosely(t, treeClosers, should.HaveLength(1))
		tc := treeClosers[0]
		assert.Loosely(t, tc.BuilderKey, should.Resemble(datastore.MakeKey(c, "Project", "chromium", "Builder", "ci/linux")))
		assert.Loosely(t, tc.TreeName, should.Equal("chromium"))
		assert.Loosely(t, tc.Status, should.Equal(Open))
		assert.Loosely(t, &tc.TreeCloser, should.Resemble(&notifypb.TreeCloser{
			TreeStatusHost:          "chromium-status.appspot.com",
			FailedStepRegexp:        "test",
			FailedStepRegexpExclude: "experimental_test",
		}))

		// Regression test for a bug where we would incorrectly delete entities
		// that are still live in the config.
		t.Run("Entities remain after no-op update", func(t *ftt.Test) {
			// Add a space - this won't change the contents of the config, but
			// it will update the hash, hence forcing a reingestion of the same
			// config, which should be a no-op.
			cfg["projects/chromium"]["luci-notify.cfg"] += " "
			cfg["projects/v8"]["luci-notify.cfg"] += " "
			c := cfgclient.Use(c, memory.New(cfg))

			err := updateProjects(c)
			assert.Loosely(t, err, should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()

			var builders []*Builder
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("Builder"), &builders), should.BeNil)
			assert.Loosely(t, builders, should.HaveLength(2))

			var emailTemplates []*EmailTemplate
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("EmailTemplate"), &emailTemplates), should.BeNil)
			assert.Loosely(t, emailTemplates, should.HaveLength(4))

			var treeClosers []*TreeCloser
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("TreeCloser"), &treeClosers), should.BeNil)
			assert.Loosely(t, treeClosers, should.HaveLength(1))
		})

		t.Run("preserve updated fields", func(t *ftt.Test) {
			// Update the Chromium builder in the datastore, simulating that some request was handled.
			chromiumBuilder := builders[0]
			chromiumBuilder.Status = buildbucketpb.Status_FAILURE
			chromiumBuilder.Revision = "abc123"
			chromiumBuilder.GitilesCommits = &notifypb.GitilesCommits{
				Commits: []*buildbucketpb.GitilesCommit{
					{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Id:      "deadbeefdeadbeefdeadbeef",
					},
				},
			}
			assert.Loosely(t, datastore.Put(c, chromiumBuilder), should.BeNil)

			// Similar with the TreeCloser
			treeCloser := treeClosers[0]
			treeCloser.Status = Closed
			treeCloser.Timestamp = time.Now().UTC()
			assert.Loosely(t, datastore.Put(c, treeCloser), should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()

			// Force a new config revision by slightly changing the config.
			cfg["projects/chromium"]["luci-notify.cfg"] += " "
			c := cfgclient.Use(c, memory.New(cfg))

			assert.Loosely(t, updateProjects(c), should.BeNil)

			chromium := &Project{Name: "chromium"}
			chromiumKey := datastore.KeyForObj(c, chromium)

			var newBuilders []*Builder
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("Builder").Ancestor(chromiumKey), &newBuilders), should.BeNil)
			assert.Loosely(t, newBuilders, should.HaveLength(1))
			// Check the fields we care about explicitly, because generated proto structs may have
			// size caches which are updated.
			assert.Loosely(t, newBuilders[0].Status, should.Equal(chromiumBuilder.Status))
			assert.Loosely(t, newBuilders[0].Revision, should.Resemble(chromiumBuilder.Revision))
			assert.Loosely(t, newBuilders[0].GitilesCommits, should.Resemble(chromiumBuilder.GitilesCommits))

			var newTreeClosers []*TreeCloser
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("TreeCloser").Ancestor(chromiumKey), &newTreeClosers), should.BeNil)
			assert.Loosely(t, newTreeClosers, should.HaveLength(1))
			assert.Loosely(t, newTreeClosers[0].Status, should.Equal(treeCloser.Status))
			// The returned Timestamp field is rounded to the microsecond, as datastore only stores times to µs precision.
			µs, _ := time.ParseDuration("1µs")
			assert.Loosely(t, newTreeClosers[0].Timestamp, should.Match(treeCloser.Timestamp.Round(µs)))
		})
		t.Run("Large project", func(t *ftt.Test) {
			const configTemplate = `notifiers {
				name: "%v-notifier"
				notifications {
					on_new_status: SUCCESS
					on_new_status: FAILURE
					on_new_status: INFRA_FAILURE
					email {
						recipients: "johndoe@v8.org"
						recipients: "janedoe@v8.org"
					}
				}
				builders {
					bucket: "ci"
					name: "builder-%v"
				}
				tree_closers {
				  tree_status_host: "chromeos-status.appspot.com"
				}
			}
			`
			var config strings.Builder
			config.WriteString("tree_closing_enabled: true\n")
			for i := 0; i < 1000; i++ {
				config.WriteString(fmt.Sprintf(configTemplate, i, i))
			}

			cfg["projects/chromeos"] = memory.Files{
				"luci-notify.cfg": config.String(),
			}
			c := cfgclient.Use(c, memory.New(cfg))

			assert.Loosely(t, updateProjects(c), should.BeNil)

			chromeos := &Project{Name: "chromeos"}
			chromeosKey := datastore.KeyForObj(c, chromeos)

			assert.Loosely(t, datastore.Get(c, chromeos), should.BeNil)
			assert.Loosely(t, chromeos.Name, should.Equal("chromeos"))
			assert.Loosely(t, chromeos.TreeClosingEnabled, should.BeTrue)

			var builders []*Builder
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("Builder").Ancestor(chromeosKey), &builders), should.BeNil)
			assert.Loosely(t, len(builders), should.Equal(1000))

			var treeClosers []*TreeCloser
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("TreeCloser").Ancestor(chromeosKey), &treeClosers), should.BeNil)
			assert.Loosely(t, len(treeClosers), should.Equal(1000))
		})
		t.Run("delete project", func(t *ftt.Test) {
			delete(cfg, "projects/v8")
			assert.Loosely(t, updateProjects(c), should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()

			v8 := &Project{Name: "v8"}
			assert.Loosely(t, datastore.Get(c, v8), should.Equal(datastore.ErrNoSuchEntity))
			v8Key := datastore.KeyForObj(c, v8)

			var builders []*Builder
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("Builder").Ancestor(v8Key), &builders), should.BeNil)
			assert.Loosely(t, builders, should.BeEmpty)

			var emailTemplates []*EmailTemplate
			assert.Loosely(t, datastore.GetAll(c, datastore.NewQuery("EmailTemplate").Ancestor(v8Key), &emailTemplates), should.BeNil)
			assert.Loosely(t, emailTemplates, should.BeEmpty)
		})

		t.Run("rename email template", func(t *ftt.Test) {
			oldName := "luci-notify/email-templates/a.template"
			newName := "luci-notify/email-templates/c.template"
			chromiumCfg := cfg["projects/chromium"]
			chromiumCfg[newName] = chromiumCfg[oldName]
			delete(chromiumCfg, oldName)

			assert.Loosely(t, updateProjects(c), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			var emailTemplates []*EmailTemplate
			q := datastore.NewQuery("EmailTemplate").Ancestor(datastore.MakeKey(c, "Project", "chromium"))
			assert.Loosely(t, datastore.GetAll(c, q, &emailTemplates), should.BeNil)
			assert.Loosely(t, len(emailTemplates), should.Equal(2))
			assert.Loosely(t, emailTemplates[0].Name, should.Equal("b"))
			assert.Loosely(t, emailTemplates[1].Name, should.Equal("c"))
		})
	})
}
