// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"context"
	"fmt"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMirrorIterator(t *testing.T) {
	t.Parallel()

	Convey("MirrorIterator and its factory work", t, func() {
		ctx := context.Background()
		const baseHost = "a.example.com"
		Convey("No mirrors", func() {
			it := newMirrorIterator(ctx)
			So(it.Empty(), ShouldBeFalse)
			So(it.next()(baseHost), ShouldResemble, baseHost)
			So(it.Empty(), ShouldBeTrue)
			So(it.next()(baseHost), ShouldResemble, baseHost)
			So(it.Empty(), ShouldBeTrue)
			So(it.next()(baseHost), ShouldResemble, baseHost)
		})
		Convey("One mirrors", func() {
			it := newMirrorIterator(ctx, "m1-")
			So(it.Empty(), ShouldBeFalse)
			So(it.next()(baseHost), ShouldResemble, baseHost)
			So(it.Empty(), ShouldBeFalse)
			So(it.next()(baseHost), ShouldResemble, "m1-"+baseHost)
			So(it.Empty(), ShouldBeTrue)
			So(it.next()(baseHost), ShouldResemble, baseHost)
		})
		Convey("Shuffles mirrors", func() {
			prefixes := make([]string, 10)
			expectedHosts := make([]string, len(prefixes)+1)
			expectedHosts[0] = baseHost
			for i := range prefixes {
				// use "m" prefix such that its lexicographically after baseHost itself.
				p := fmt.Sprintf("m%d-", i)
				prefixes[i] = p
				expectedHosts[i+1] = p + baseHost
			}
			iterate := func() []string {
				var actual []string
				it := newMirrorIterator(ctx, prefixes...)
				for !it.Empty() {
					actual = append(actual, it.next()(baseHost))
				}
				return actual
			}
			act1 := iterate()
			So(act1, ShouldNotResemble, expectedHosts)
			act2 := iterate()
			So(act2, ShouldNotResemble, expectedHosts)
			So(act1, ShouldNotResemble, act2)

			sort.Strings(act1)
			So(act1, ShouldResemble, expectedHosts)
			sort.Strings(act2)
			So(act2, ShouldResemble, expectedHosts)
		})
	})
}
