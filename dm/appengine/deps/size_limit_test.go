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

package deps

import (
	"errors"
	"math"
	"runtime"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/sync/parallel"
)

func TestSizeLimit(t *testing.T) {
	t.Parallel()

	Convey("Test SizeLimit", t, func() {
		Convey("no max", func() {
			l := sizeLimit{}
			So(l.PossiblyOK(100), ShouldBeTrue)
			So(l.Add(100), ShouldBeTrue)
		})

		Convey("some max", func() {
			l := sizeLimit{max: 100}
			So(l.PossiblyOK(100), ShouldBeTrue)
			So(l.PossiblyOK(101), ShouldBeFalse)
			So(l.Add(101), ShouldBeFalse)
			So(l.Add(20), ShouldBeTrue)
			So(l.PossiblyOK(100), ShouldBeFalse)
			So(l.Add(20), ShouldBeTrue)
			So(l.Add(20), ShouldBeTrue)
			So(l.Add(20), ShouldBeTrue)
			So(l.Add(20), ShouldBeTrue)
			So(l.Add(20), ShouldBeFalse)
		})

		Convey("check overflow", func() {
			l := sizeLimit{current: math.MaxUint32 - 5, max: math.MaxUint32}
			So(l.PossiblyOK(math.MaxUint32), ShouldBeFalse)
			So(l.Add(math.MaxUint32), ShouldBeFalse)
			So(l.PossiblyOK(5), ShouldBeTrue)
			So(l.PossiblyOK(6), ShouldBeFalse)
			So(l.Add(4), ShouldBeTrue)
			So(l.PossiblyOK(5), ShouldBeFalse)
			So(l.PossiblyOK(1), ShouldBeTrue)
			So(l.Add(1), ShouldBeTrue)
			So(l.Add(1), ShouldBeFalse)
			So(l.Add(math.MaxUint32), ShouldBeFalse)
			So(l.PossiblyOK(1), ShouldBeFalse)
		})

		Convey("concurrency", func() {
			prev := runtime.GOMAXPROCS(512)
			defer runtime.GOMAXPROCS(prev)
			sl := sizeLimit{max: 512}
			err := parallel.FanOutIn(func(pool chan<- func() error) {
				for i := 0; i < 512; i++ {
					pool <- func() error {
						if !sl.Add(1) {
							return errors.New("failed to add")
						}
						return nil
					}
				}
			})
			So(err, ShouldBeNil)
			So(sl.current, ShouldEqual, 512)
		})
	})
}
