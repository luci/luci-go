// Copyright 2016 The LUCI Authors.
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

package frontend

import (
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

func searchHandler(c *router.Context) {
	s := ui.Search{}
	var mBuildbot, mBuildbucket *ui.CIService

	err := parallel.FanOutIn(func(ch chan<- func() error) {
		ch <- func() (err error) {
			mBuildbot, err = buildbot.GetAllBuilders(c.Context)
			return err
		}
		ch <- func() (err error) {
			mBuildbucket, err = buildbucket.GetAllBuilders(c.Context)
			return err
		}
	})

	s.CIServices = append(s.CIServices, *mBuildbucket)
	s.CIServices = append(s.CIServices, *mBuildbot)
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	templates.MustRender(c.Context, c.Writer, "pages/search.html", templates.Args{
		"search": s,
		"error":  errMsg,
	})
}

func searchTestData() []common.TestBundle {
	return []common.TestBundle{
		{
			Description: "Basic frontpage",
		},
	}
}
