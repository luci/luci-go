// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAffectednessSlice(t *testing.T) {
	t.Parallel()
	Convey(`TestAffectednessSlice`, t, func() {
		Convey(`closest`, func() {
			Convey(`Works`, func() {
				s := AffectednessSlice{
					{Distance: 1, Rank: 2},
					{Distance: 0, Rank: 1},
				}
				i, err := s.closest()
				So(err, ShouldBeNil)
				So(i, ShouldEqual, 1)
			})

			Convey(`Rank diff`, func() {
				s := AffectednessSlice{
					{Distance: 0, Rank: 2},
					{Distance: 0, Rank: 1},
					{Distance: 0, Rank: 3},
				}
				i, err := s.closest()
				So(err, ShouldBeNil)
				So(i, ShouldEqual, 1)
			})

			Convey(`Empty`, func() {
				_, err := AffectednessSlice(nil).closest()
				So(err, ShouldErrLike, "empty")
			})

			Convey(`Single`, func() {
				s := AffectednessSlice{{Distance: 0}}
				i, err := s.closest()
				So(err, ShouldBeNil)
				So(i, ShouldEqual, 0)
			})

			Convey(`Inconsistent`, func() {
				s := AffectednessSlice{
					{Distance: 0, Rank: 2},
					{Distance: 1, Rank: 1},
				}
				_, err := s.closest()
				So(err, ShouldErrLike, `ranks and distances are inconsistent`)
			})
		})
	})

	Convey(`checkConsistency`, t, func() {
		a := Affectedness{Distance: 0, Rank: 2}
		b := Affectedness{Distance: 1, Rank: 1}
		So(checkConsistency(a, b), ShouldErrLike, `ranks and distances are inconsistent: eval.Affectedness{Distance:0, Rank:2} and eval.Affectedness{Distance:1, Rank:1}`)
		So(checkConsistency(b, a), ShouldErrLike, `ranks and distances are inconsistent: eval.Affectedness{Distance:1, Rank:1} and eval.Affectedness{Distance:0, Rank:2}`)
	})
}

func TestQuantiles(t *testing.T) {
	t.Parallel()
	Convey(`Quantiles`, t, func() {
		Convey(`median of 1, 2, 3, 4`, func() {
			afs := AffectednessSlice{
				Affectedness{Distance: 1, Rank: 1},
				Affectedness{Distance: 2, Rank: 2},
				Affectedness{Distance: 3, Rank: 3},
				Affectedness{Distance: 4, Rank: 4},
			}
			distances, ranks := afs.quantiles(2)
			So(distances, ShouldResemble, []float64{2, 4})
			So(ranks, ShouldResemble, []int{2, 4})
		})
		Convey(`4-quantiles of 1, 2, 3, 4`, func() {
			afs := AffectednessSlice{
				Affectedness{Distance: 1},
				Affectedness{Distance: 2},
				Affectedness{Distance: 3},
				Affectedness{Distance: 4},
			}
			distances, _ := afs.quantiles(4)
			So(distances, ShouldResemble, []float64{1, 2, 3, 4})
		})
		Convey(`10-quantiles of 1, 2, 3, 4`, func() {
			afs := AffectednessSlice{
				Affectedness{Distance: 1},
				Affectedness{Distance: 2},
				Affectedness{Distance: 3},
				Affectedness{Distance: 4},
			}
			distances, _ := afs.quantiles(10)
			So(distances, ShouldResemble, []float64{1, 1, 2, 2, 2, 3, 3, 4, 4, 4})
		})
	})
}
