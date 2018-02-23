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

package config

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigSet(t *testing.T) {
	t.Parallel()

	Convey(`Testing config set utility methods`, t, func() {
		So(ServiceSet("my-service"), ShouldEqual, "services/my-service")
		So(ProjectSet("my-project"), ShouldEqual, "projects/my-project")
		So(RefSet("my-project", "refs/heads/master"), ShouldEqual, "projects/my-project/refs/heads/master")

		project, configSet, tail := Set("projects/foo").SplitProject()
		So(project, ShouldEqual, "foo")
		So(configSet, ShouldEqual, "projects/foo")
		So(tail, ShouldEqual, "")

		project, configSet, tail = Set("projects/foo/refs/heads/master").SplitProject()
		So(project, ShouldEqual, "foo")
		So(configSet, ShouldEqual, "projects/foo")
		So(tail, ShouldEqual, "refs/heads/master")

		project, configSet, tail = Set("not/a/project/config/set").SplitProject()
		So(project, ShouldEqual, "")
		So(configSet, ShouldEqual, "")
		So(tail, ShouldEqual, "")
	})
}
