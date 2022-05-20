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

func TestGerritQueryString(t *testing.T) {
	t.Parallel()

	Convey("gerritString works", t, func() {
		qs := &QueryState{}

		Convey("single project", func() {
			qs.OrProjects = []string{"inf/ra"}
			So(qs.gerritString(queryLimited), ShouldEqual,
				`status:NEW project:"inf/ra"`)
		})

		Convey("many projects", func() {
			qs.OrProjects = []string{"inf/ra", "second"}
			So(qs.gerritString(queryLimited), ShouldEqual,
				`status:NEW (project:"inf/ra" OR project:"second")`)
		})

		Convey("shared prefix", func() {
			qs.CommonProjectPrefix = "shared"
			So(qs.gerritString(queryLimited), ShouldEqual,
				`status:NEW projects:"shared"`)
		})

		Convey("unlimited", func() {
			qs.CommonProjectPrefix = "shared"
			So(qs.gerritString(queryAll), ShouldEqual,
				`projects:"shared"`)
		})
	})
}
