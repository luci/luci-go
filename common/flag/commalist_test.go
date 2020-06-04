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

package flag

import (
	"flag"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCommaList(t *testing.T) {
	t.Parallel()
	Convey("Given a FlagSet with a CommaList flag", t, func() {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Usage = func() {}
		fs.SetOutput(ioutil.Discard)
		var s []string
		fs.Var(CommaList(&s), "list", "Some list")
		Convey("When parsing with flag absent", func() {
			err := fs.Parse([]string{})
			Convey("The parsed slice should be empty", func() {
				So(err, ShouldEqual, nil)
				So(len(s), ShouldEqual, 0)
			})
		})
		Convey("When parsing a single item", func() {
			err := fs.Parse([]string{"-list", "foo"})
			Convey("The parsed slice should contain the item", func() {
				So(err, ShouldEqual, nil)
				So(s, ShouldResemble, []string{"foo"})
			})
		})
		Convey("When parsing multiple items", func() {
			err := fs.Parse([]string{"-list", "foo,bar,spam"})
			Convey("The parsed slice should contain the items", func() {
				So(err, ShouldEqual, nil)
				So(s, ShouldResemble, []string{"foo", "bar", "spam"})
			})
		})
	})
}
