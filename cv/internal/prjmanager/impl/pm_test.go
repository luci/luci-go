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
	"time"

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq/tqtesting"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/gerrit/poller/pollertest"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/pmtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUpdateConfig(t *testing.T) {
	t.Parallel()

	Convey("UpdateConfig works alone", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		lProjectKey := datastore.MakeKey(ctx, prjmanager.ProjectKind, lProject)

		So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
		Convey("with new project", func() {
			ct.Cfg.Create(ctx, lProject, singleRepoConfig("host", "repo"))
			So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
			So(pmtest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
			ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-pm-task"))
			events, err := eventbox.List(ctx, lProjectKey)
			So(err, ShouldBeNil)
			So(events, ShouldHaveLength, 0)
			p := getProject(ctx, lProject)
			So(p.EVersion, ShouldEqual, 1)
			So(p.Status, ShouldEqual, prjmanager.Status_STARTED)
			So(pollertest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})

			Convey("... and just deleted project", func() {
				ct.Clock.Add(time.Hour) // ensure first poller task gets executed.
				ct.Cfg.Delete(ctx, lProject)
				So(prjmanager.UpdateConfig(ctx, lProject), ShouldBeNil)
				ct.TQ.Run(ctx, tqtesting.StopAfterTask("poke-pm-task"))
				p := getProject(ctx, lProject)
				So(p.EVersion, ShouldEqual, 2)
				So(p.Status, ShouldEqual, prjmanager.Status_STOPPED)
				So(pollertest.Projects(ct.TQ.Tasks()), ShouldResemble, []string{lProject})
			})
		})
	})
}

func getProject(ctx context.Context, luciProject string) *prjmanager.Project {
	p := &prjmanager.Project{ID: luciProject}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return nil
	default:
		So(err, ShouldBeNil)
		return p
	}
}

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
