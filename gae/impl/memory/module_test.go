// Copyright 2015 The LUCI Authors.
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

package memory

import (
	"testing"

	"go.chromium.org/gae/service/module"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestModule(t *testing.T) {
	Convey("NumInstances", t, func() {
		c := Use(context.Background())

		i, err := module.NumInstances(c, "foo", "bar")
		So(i, ShouldEqual, 1)
		So(err, ShouldBeNil)

		So(module.SetNumInstances(c, "foo", "bar", 42), ShouldBeNil)
		i, err = module.NumInstances(c, "foo", "bar")
		So(i, ShouldEqual, 42)
		So(err, ShouldBeNil)

		i, err = module.NumInstances(c, "foo", "baz")
		So(i, ShouldEqual, 1)
		So(err, ShouldBeNil)
	})
}
