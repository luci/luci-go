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

package casclient

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseCASInstance(t *testing.T) {
	t.Parallel()

	Convey(`Basic`, t, func() {
		ins, err := parseCASInstance("abc-123")
		So(err, ShouldBeNil)
		So(ins, ShouldResemble, "projects/abc-123/instances/default_instance")

		ins, err = parseCASInstance("projects/foo/instances/bar")
		So(err, ShouldBeNil)
		So(ins, ShouldResemble, "projects/foo/instances/bar")
	})

	Convey(`Invalid`, t, func() {
		_, err := parseCASInstance("ABC")
		So(err, ShouldNotBeNil)

		_, err = parseCASInstance("projects/foo")
		So(err, ShouldNotBeNil)

		_, err = parseCASInstance("projects/foo/instances/bar/42")
		So(err, ShouldNotBeNil)
	})
}
