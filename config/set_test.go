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
		ss, err := ServiceSet("my-service")
		So(err, ShouldBeNil)
		So(ss, ShouldEqual, "services/my-service")
		ps, err := ProjectSet("my-project")
		So(err, ShouldBeNil)
		So(ps, ShouldEqual, "projects/my-project")

		s := MustServiceSet("foo")
		So(s.Service(), ShouldEqual, "foo")
		So(s.Project(), ShouldEqual, "")

		s = MustProjectSet("foo")
		So(s.Service(), ShouldEqual, "")
		So(s.Project(), ShouldEqual, "foo")

		s = Set("malformed/set/abc/def")
		So(s.Service(), ShouldEqual, "")
		So(s.Project(), ShouldEqual, "")

		s, err = ServiceSet("malformed/service/set/abc")
		So(err, ShouldNotBeNil)
		So(s, ShouldBeEmpty)

		s, err = ServiceSet("malformed/project/set/abc")
		So(err, ShouldNotBeNil)
		So(s, ShouldBeEmpty)
	})
}
