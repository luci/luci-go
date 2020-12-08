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

package impl

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEventPipeline(t *testing.T) {
	t.Parallel()

	Convey("Event pipeline works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"

		test := func(trans bool) {
			poke := func() {
				if trans {
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return prjmanager.UpdateConfig(ctx, lProject)
					}, nil), ShouldBeNil)
				} else {
					So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
				}
			}
			Convey("with new project", func() {
				ct.Cfg.Create(ctx, lProject, singleRepoConfig("host", "repo"))
				poke()
				So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 1)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-pm-task"))
				So(ct.TQ.Tasks().Payloads(), ShouldHaveLength, 0)
				events, err := internal.Peek(ctx, lProject, 100)
				So(err, ShouldBeNil)
				So(events, ShouldHaveLength, 0)
				p := &project{ID: lProject}
				So(datastore.Get(ctx, p), ShouldBeNil)
				So(p.EVersion, ShouldEqual, 1)
				So(p.State, ShouldEqual, PMState_PM_STATE_STARTED)

				Convey("... and just deleted project", func() {
					ct.Cfg.Delete(ctx, lProject)
					poke()
					ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-pm-task"))
					So(datastore.Get(ctx, p), ShouldBeNil)
					So(p.EVersion, ShouldEqual, 2)
					So(p.State, ShouldEqual, PMState_PM_STATE_STOPPED)
				})
			})
		}
		Convey("Non-Transactional", func() {
			test(false)
		})
		Convey("Transactional", func() {
			test(true)
		})
	})
}

// TODO(tandrii): add proper test for UpdateConfig.

func singleRepoConfig(gHost string, gRepos ...string) *cfgpb.Config {
	projects := make([]*cfgpb.ConfigGroup_Gerrit_Project, len(gRepos))
	for i, gRepo := range gRepos {
		projects[i] = &cfgpb.ConfigGroup_Gerrit_Project{
			Name:      gRepo,
			RefRegexp: []string{"refs/heads/main"},
		}
	}
	return &cfgpb.Config{
		ConfigGroups: []*cfgpb.ConfigGroup{
			{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url:      "https://" + gHost + "/",
						Projects: projects,
					},
				},
			},
		},
	}
}
