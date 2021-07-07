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

package poller

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildQuery(t *testing.T) {
	t.Parallel()

	Convey("buildQuery works", t, func() {
		sp := &SubPoller{}

		Convey("single project", func() {
			sp.OrProjects = []string{"inf/ra"}
			So(buildQuery(sp, queryLimited), ShouldEqual,
				`status:NEW label:Commit-Queue>0 project:"inf/ra"`)
		})

		Convey("many projects", func() {
			sp.OrProjects = []string{"inf/ra", "second"}
			So(buildQuery(sp, queryLimited), ShouldEqual,
				`status:NEW label:Commit-Queue>0 (project:"inf/ra" OR project:"second")`)
		})

		Convey("shared prefix", func() {
			sp.CommonProjectPrefix = "shared"
			So(buildQuery(sp, queryLimited), ShouldEqual,
				`status:NEW label:Commit-Queue>0 projects:"shared"`)
		})

		Convey("unlimited", func() {
			sp.CommonProjectPrefix = "shared"
			So(buildQuery(sp, queryAll), ShouldEqual,
				`projects:"shared"`)
		})
	})
}
