// Copyright 2018 The LUCI Authors.
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

package main

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubstitute(t *testing.T) {
	t.Parallel()

	Convey("substitute", t, func() {
		c := context.Background()
		subs := map[string]string{
			"Field1": "Value1",
			"Field2": "Value2",
			"Field3": "Value3",
		}

		Convey("err", func() {
			s, err := substitute(c, "Test {{", subs)
			So(err, ShouldNotBeNil)
			So(s, ShouldEqual, "")
		})

		Convey("partial", func() {
			s, err := substitute(c, "Test {{.Field2}}", subs)
			So(err, ShouldBeNil)
			So(s, ShouldEqual, "Test Value2")
		})

		Convey("full", func() {
			s, err := substitute(c, "Test {{.Field1}} {{.Field2}} {{.Field3}}", subs)
			So(err, ShouldBeNil)
			So(s, ShouldEqual, "Test Value1 Value2 Value3")
		})
	})
}
