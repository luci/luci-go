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
		Convey("Dedupe", func() {
			v := []int64{7, 6, 3, 1, 3, 4, 9, 2, 1, 5, 8, 8, 8, 4, 9}
			v1 := UniqueSorted(v)
			So(v1, ShouldResemble, []int64{1, 2, 3, 4, 5, 6, 7, 8, 9})
			So(v[:len(v1)], ShouldResemble, v1) // re-uses space
		})
	})
}
