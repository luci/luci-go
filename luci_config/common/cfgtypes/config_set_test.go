// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cfgtypes

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigSet(t *testing.T) {
	t.Parallel()

	Convey(`Testing config set utility methods`, t, func() {
		So(ServiceConfigSet("my-service"), ShouldEqual, "services/my-service")
		So(ProjectConfigSet("my-project"), ShouldEqual, "projects/my-project")
		So(RefConfigSet("my-project", "refs/heads/master"), ShouldEqual, "projects/my-project/refs/heads/master")

		project, configSet, tail := ConfigSet("projects/foo").SplitProject()
		So(project, ShouldEqual, "foo")
		So(configSet, ShouldEqual, "projects/foo")
		So(tail, ShouldEqual, "")

		project, configSet, tail = ConfigSet("projects/foo/refs/heads/master").SplitProject()
		So(project, ShouldEqual, "foo")
		So(configSet, ShouldEqual, "projects/foo")
		So(tail, ShouldEqual, "refs/heads/master")

		project, configSet, tail = ConfigSet("not/a/project/config/set").SplitProject()
		So(project, ShouldEqual, "")
		So(configSet, ShouldEqual, "")
		So(tail, ShouldEqual, "")
	})
}
