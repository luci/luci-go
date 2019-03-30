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

package main

import (
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseProperties(t *testing.T) {
	Convey("ParseProperties", t, func() {

		f, err := ioutil.TempFile("", "")
		So(err, ShouldBeNil)
		defer f.Close()

		f.WriteString(`{
			"in-file-1": "orig",
			"in-file-2": "orig"
		}`)

		r := &addRun{
			properties: []string{
				"@" + f.Name(),
				"in-file-2=override",

				"int=1",
				"str=b",
				"array=[1]",
			},
		}

		actual, err := r.parseProperties()
		So(err, ShouldBeNil)
		So(actual, shouldResembleJSONPB, `{
			"in-file-1": "orig",
			"in-file-2": "override",
			"int": 1,
			"str": "b",
			"array": [1]
		}`)
	})
}
