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

package environ

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEnvironment(t *testing.T) {
	Convey(`Environment tests`, t, func() {
		Convey(`An empty enviornment array yields a nil enviornment.`, func() {
			So(Load([]string{}), ShouldBeNil)
		})

		Convey(`An environment consisting of KEY and KEY=VALUE pairs should load correctly.`, func() {
			So(Load([]string{
				"",
				"FOO",
				"BAR=",
				"BAZ=QUX",
				"=QUUX",
			}), ShouldResemble, Environment{
				"FOO": "",
				"BAR": "",
				"BAZ": "QUX",
			})
		})
	})
}
