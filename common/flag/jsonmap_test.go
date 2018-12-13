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

func TestJSONMap(t *testing.T) {
	t.Parallel()
	Convey("Given a FlagSet with a JSONMap flag", t, func() {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Usage = func() {}
		fs.SetOutput(ioutil.Discard)
		var m map[string]string
		fs.Var(NewJSONMap(&m), "map", "Some map")
		Convey("When parsing a JSON object", func() {
			err := fs.Parse([]string{"-map", `{"foo": "bar"}`})
			Convey("The parsed map should match the object", func() {
				So(err, ShouldEqual, nil)
				So(m, ShouldResemble, map[string]string{"foo": "bar"})
			})
		})
	})
}
