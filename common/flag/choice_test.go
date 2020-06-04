// Copyright 2019 The LUCI Authors.
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

package flag

import (
	"flag"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChoice(t *testing.T) {
	t.Parallel()
	Convey("Given a FlagSet with a Choice flag", t, func() {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Usage = func() {}
		fs.SetOutput(ioutil.Discard)
		var s string
		fs.Var(NewChoice(&s, "Apple", "Orange"), "fruit", "")
		Convey("When parsing with flag absent", func() {
			err := fs.Parse([]string{})
			Convey("The parsed string should be empty", func() {
				So(err, ShouldBeNil)
				So(s, ShouldEqual, "")
			})
		})
		Convey("When parsing a valid choice", func() {
			err := fs.Parse([]string{"-fruit", "Orange"})
			Convey("The parsed flag equals that choice", func() {
				So(err, ShouldBeNil)
				So(s, ShouldEqual, "Orange")
			})
		})
		Convey("When parsing an invalid choice", func() {
			err := fs.Parse([]string{"-fruit", "Onion"})
			Convey("An error is returned", func() {
				So(err, ShouldNotBeNil)
				So(s, ShouldEqual, "")
			})
		})
	})
}
