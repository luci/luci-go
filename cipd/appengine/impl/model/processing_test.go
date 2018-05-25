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

package model

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProcessingResult(t *testing.T) {
	t.Parallel()

	res := map[string]string{
		"a": "b",
		"c": "d",
	}

	Convey("Read/Write result works", t, func() {
		p := ProcessingResult{}

		So(p.WriteResult(res), ShouldBeNil)

		out := map[string]string{}
		So(p.ReadResult(&out), ShouldBeNil)
		So(out, ShouldResemble, res)
	})
}
