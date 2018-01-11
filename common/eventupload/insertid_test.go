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

package eventupload

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerate(t *testing.T) {
	t.Parallel()

	prefix := "testPrefix"
	id := InsertIDGenerator{}
	id.Prefix = prefix

	Convey("Test InsertIDGenerator increments counter with calls to Generate", t, func() {
		Convey("Test InsertIDGenerator increments counter with calls to Generate", func() {
			for i := 1; i < 10; i++ {
				want := fmt.Sprintf("%s:%d", prefix, i)
				Convey(fmt.Sprintf("When Generate is called %d time(s), the value of the counter is correct", i), func() {
					got := id.Generate()
					So(got, ShouldEqual, want)
				})
			}
		})
	})
}
