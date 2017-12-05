// Copyright 2017 The LUCI Authors.
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

package isolated

import (
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScatterGatherAdd(t *testing.T) {
	t.Parallel()

	wd1 := filepath.Join("tmp", "go")
	wd2 := filepath.Join("tmp", "stop")
	rp1 := "ha"
	rp2 := filepath.Join("hah", "bah")
	rp3 := filepath.Join("hah", "nah")
	rp3unclean := rp3 + "/"

	Convey(`Test that Add works in a good case.`, t, func() {
		sc := ScatterGather{}
		So(sc.Add(wd1, rp1), ShouldBeNil)
		So(sc.Add(wd1, rp2), ShouldBeNil)
		So(sc.Add(wd2, rp3unclean), ShouldBeNil)

		So(sc, ShouldResemble, ScatterGather{
			rp1: wd1,
			rp2: wd1,
			rp3: wd2,
		})
	})

	Convey(`Test that Add fails in a bad case.`, t, func() {
		sc := ScatterGather{}
		So(sc.Add(wd1, rp1), ShouldBeNil)
		So(sc.Add(wd1, rp1), ShouldNotBeNil)
		So(sc.Add(wd2, rp1), ShouldNotBeNil)
		So(sc.Add(wd2, rp3), ShouldBeNil)
		So(sc.Add(wd1, rp3unclean), ShouldNotBeNil)
	})
}
