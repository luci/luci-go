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

package clock

import (
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTags(t *testing.T) {
	Convey(`An empty Context`, t, func() {
		c := context.Background()

		Convey(`Should have nil tags.`, func() {
			So(Tags(c), ShouldBeNil)
		})

		Convey(`With tag, "A"`, func() {
			c = Tag(c, "A")

			Convey(`Should have tags {"A"}.`, func() {
				So(Tags(c), ShouldResemble, []string{"A"})
			})

			Convey(`And another tag, "B"`, func() {
				c = Tag(c, "B")

				Convey(`Should have tags {"A", "B"}.`, func() {
					So(Tags(c), ShouldResemble, []string{"A", "B"})
				})
			})
		})
	})
}
