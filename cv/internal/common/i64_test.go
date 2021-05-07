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

package common

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestI64s(t *testing.T) {
	t.Parallel()

	Convey("UniqueSorted", t, func() {
		v := []int64{7, 6, 3, 1, 3, 4, 9, 2, 1, 5, 8, 8, 8, 4, 9}
		v1 := UniqueSorted(v)
		So(v1, ShouldResemble, []int64{1, 2, 3, 4, 5, 6, 7, 8, 9})
		So(v[:len(v1)], ShouldResemble, v1) // re-uses space

		v = []int64{1, 2, 2, 3, 4, 5, 5}
		v1 = UniqueSorted(v)
		So(v1, ShouldResemble, []int64{1, 2, 3, 4, 5})
		So(v[:len(v1)], ShouldResemble, v1) // re-uses space
	})

	Convey("DifferenceSorted", t, func() {
		all := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
		odd := []int64{1, 3, 5, 7, 9, 11}
		div3 := []int64{3, 6, 9}

		So(DifferenceSorted(odd, all), ShouldBeEmpty)
		So(DifferenceSorted(div3, all), ShouldBeEmpty)

		So(DifferenceSorted(all, odd), ShouldResemble, []int64{2, 4, 6, 8, 10})
		So(DifferenceSorted(all, div3), ShouldResemble, []int64{1, 2, 4, 5, 7, 8, 10, 11})

		So(DifferenceSorted(odd, div3), ShouldResemble, []int64{1, 5, 7, 11})
		So(DifferenceSorted(div3, odd), ShouldResemble, []int64{6})
	})

	Convey("UnionSorted", t, func() {
		all := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9}
		odd := []int64{1, 3, 5, 7, 9}
		div3 := []int64{3, 6, 9}

		So(UnionSorted(odd, all), ShouldResemble, all)
		So(UnionSorted(all, div3), ShouldResemble, all)
		So(UnionSorted(all, all), ShouldResemble, all)
		So(UnionSorted(all, nil), ShouldResemble, all)
		So(UnionSorted(nil, all), ShouldResemble, all)

		So(UnionSorted(odd, div3), ShouldResemble, []int64{1, 3, 5, 6, 7, 9})
		So(UnionSorted(div3, odd), ShouldResemble, []int64{1, 3, 5, 6, 7, 9})
	})
}
