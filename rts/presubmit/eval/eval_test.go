// Copyright 2020The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0(the "License");
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
	"bytes"
	"container/heap"
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/rts"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGridBuckets(t *testing.T) {
	t.Parallel()
	Convey(`gridBuckets`, t, func() {
		var b bucketGrid

		Convey(`inc`, func() {
			afs := make([]rts.Affectedness, 10)
			for i := 0; i < len(afs); i++ {
				afs[i] = rts.Affectedness{Distance: float64(i + 1), Rank: i + 1}
			}
			var g thresholdGrid
			g.init(afs)

			Convey(`(2, 2)`, func() {
				b.inc(&g, rts.Affectedness{Distance: 2, Rank: 2}, 1)
				So(g[8][0].Value.Distance, ShouldEqual, 1)
				So(g[9][0].Value.Distance, ShouldEqual, 1)
				So(g[10][0].Value.Distance, ShouldEqual, 2)
				So(b[10][10], ShouldEqual, 1)
			})
			Convey(`(3, 3)`, func() {
				b.inc(&g, rts.Affectedness{Distance: 3, Rank: 3}, 1)
				So(g[18][0].Value.Distance, ShouldEqual, 2)
				So(g[19][0].Value.Distance, ShouldEqual, 2)
				So(g[20][0].Value.Distance, ShouldEqual, 3)
				So(b[20][20], ShouldEqual, 1)
			})
			Convey(`(9, 9)`, func() {
				b.inc(&g, rts.Affectedness{Distance: 9, Rank: 9}, 1)
				So(g[78][0].Value.Distance, ShouldEqual, 8)
				So(g[79][0].Value.Distance, ShouldEqual, 8)
				So(g[80][0].Value.Distance, ShouldEqual, 9)
				So(b[80][80], ShouldEqual, 1)
			})
			Convey(`(10, 10)`, func() {
				b.inc(&g, rts.Affectedness{Distance: 10, Rank: 10}, 1)
				So(g[88][0].Value.Distance, ShouldEqual, 9)
				So(g[89][0].Value.Distance, ShouldEqual, 9)
				So(g[90][0].Value.Distance, ShouldEqual, 10)
				So(b[90][90], ShouldEqual, 1)
			})
			Convey(`(0, 0)`, func() {
				// This data point was not lost by any threshold.
				b.inc(&g, rts.Affectedness{Distance: 0, Rank: 0}, 1)
				So(b[0][0], ShouldEqual, 1)
			})
			Convey(`(11, 11)`, func() {
				// This data point was lost by all thresholds.
				b.inc(&g, rts.Affectedness{Distance: 11, Rank: 11}, 1)
				So(b[100][100], ShouldEqual, 1)
			})
		})
		Convey(`makeCumulative`, func() {
			assert10 := func(expected string) {
				expected = strings.TrimPrefix(expected, "\n")
				expected = strings.Replace(expected, "\t", "", -1)

				var buf bytes.Buffer
				for i := 0; i < 10; i++ {
					for j := 0; j < 10; j++ {
						fmt.Fprintf(&buf, "%d", b[i][j])
					}
					fmt.Fprintln(&buf)
				}
				So(buf.String(), ShouldEqual, expected)
			}

			Convey(`b[0][0] = 1`, func() {
				b[0][0] = 1
				b.makeCumulative()
				assert10(`
					1000000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
				`)
			})

			Convey(`b[5][5] = 1`, func() {
				b[5][5] = 1
				b.makeCumulative()
				assert10(`
					1111110000
					1111110000
					1111110000
					1111110000
					1111110000
					1111110000
					0000000000
					0000000000
					0000000000
					0000000000
				`)
			})

			Convey(`b[2][5] = 2`, func() {
				b[2][5] = 2
				b.makeCumulative()
				assert10(`
					2222220000
					2222220000
					2222220000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
				`)
			})

			Convey(`b[2][4] = 1, b[4][2] = 2`, func() {
				b[2][4] = 1
				b[4][2] = 2
				b.makeCumulative()
				assert10(`
					3331100000
					3331100000
					3331100000
					2220000000
					2220000000
					0000000000
					0000000000
					0000000000
					0000000000
					0000000000
				`)
			})
		})
	})
}

func TestMostAffected(t *testing.T) {
	t.Parallel()
	Convey(`Test[]rts.Affectedness`, t, func() {
		Convey(`mostAffected`, func() {
			Convey(`Works`, func() {
				most, err := mostAffected([]rts.Affectedness{
					{Distance: 1, Rank: 2},
					{Distance: 0, Rank: 1},
				})
				So(err, ShouldBeNil)
				So(most, ShouldResemble, rts.Affectedness{Distance: 0, Rank: 1})
			})

			Convey(`Rank diff`, func() {
				most, err := mostAffected([]rts.Affectedness{
					{Distance: 0, Rank: 2},
					{Distance: 0, Rank: 1},
					{Distance: 0, Rank: 3},
				})
				So(err, ShouldBeNil)
				So(most, ShouldResemble, rts.Affectedness{Distance: 0, Rank: 1})
			})

			Convey(`Empty`, func() {
				_, err := mostAffected(nil)
				So(err, ShouldErrLike, "empty")
			})

			Convey(`Single`, func() {
				most, err := mostAffected([]rts.Affectedness{{Distance: 0}})
				So(err, ShouldBeNil)
				So(most, ShouldResemble, rts.Affectedness{Distance: 0})
			})

			Convey(`Inconsistent`, func() {
				s := []rts.Affectedness{
					{Distance: 0, Rank: 2},
					{Distance: 1, Rank: 1},
				}
				_, err := mostAffected(s)
				So(err, ShouldErrLike, `ranks and distances are inconsistent`)
			})
		})
	})

	Convey(`checkConsistency`, t, func() {
		a := rts.Affectedness{Distance: 0, Rank: 2}
		b := rts.Affectedness{Distance: 1, Rank: 1}
		So(checkConsistency(a, b), ShouldErrLike, `ranks and distances are inconsistent: rts.Affectedness{Distance:0, Rank:2} and rts.Affectedness{Distance:1, Rank:1}`)
		So(checkConsistency(b, a), ShouldErrLike, `ranks and distances are inconsistent: rts.Affectedness{Distance:1, Rank:1} and rts.Affectedness{Distance:0, Rank:2}`)
	})
}

func TestQuantiles(t *testing.T) {
	t.Parallel()
	Convey(`Quantiles`, t, func() {
		Convey(`median of 1, 2, 3, 4`, func() {
			afs := []rts.Affectedness{
				{Distance: 1, Rank: 1},
				{Distance: 2, Rank: 2},
				{Distance: 3, Rank: 3},
				{Distance: 4, Rank: 4},
			}
			distances, ranks := affectednessQuantiles(afs, 2)
			So(distances, ShouldResemble, []float64{2, 4})
			So(ranks, ShouldResemble, []int{2, 4})
		})
		Convey(`4-quantiles of 1, 2, 3, 4`, func() {
			afs := []rts.Affectedness{
				{Distance: 1},
				{Distance: 2},
				{Distance: 3},
				{Distance: 4},
			}
			distances, _ := affectednessQuantiles(afs, 4)
			So(distances, ShouldResemble, []float64{1, 2, 3, 4})
		})
		Convey(`10-quantiles of 1, 2, 3, 4`, func() {
			afs := []rts.Affectedness{
				{Distance: 1},
				{Distance: 2},
				{Distance: 3},
				{Distance: 4},
			}
			distances, _ := affectednessQuantiles(afs, 10)
			So(distances, ShouldResemble, []float64{1, 1, 2, 2, 2, 3, 3, 4, 4, 4})
		})
	})
}

func TestThresholdGrid(t *testing.T) {
	Convey("ThresholGrid", t, func() {
		g := thresholdGrid{}
		g[0][0].Savings = 1

		g[98][0].ChangeRecall = 0.9

		g[98][98].ChangeRecall = 0.9
		g[98][98].Savings = 0.3

		g[99][99].ChangeRecall = 1

		So(g.Slice(), ShouldResemble, []*Threshold{
			{ChangeRecall: 0, Savings: 1.0},
			{ChangeRecall: 0.9, Savings: 0.3},
			{ChangeRecall: 1, Savings: 0.0},
		})
	})
}

func TestFurthestRejections(t *testing.T) {
	Convey("FurthestRejections", t, func() {
		furthest := make(furthestRejections, 3)
		furthest.Consider(affectedRejection{MostAffected: rts.Affectedness{Distance: 1}})
		furthest.Consider(affectedRejection{MostAffected: rts.Affectedness{Distance: 2}})
		furthest.Consider(affectedRejection{MostAffected: rts.Affectedness{Distance: 3}})
		furthest.Consider(affectedRejection{MostAffected: rts.Affectedness{Distance: 4}})
		furthest.Consider(affectedRejection{MostAffected: rts.Affectedness{Distance: 5}})

		So(len(furthest), ShouldEqual, 3)
		So(heap.Pop(&furthest), ShouldResemble, affectedRejection{MostAffected: rts.Affectedness{Distance: 3}})
		So(heap.Pop(&furthest), ShouldResemble, affectedRejection{MostAffected: rts.Affectedness{Distance: 4}})
		So(heap.Pop(&furthest), ShouldResemble, affectedRejection{MostAffected: rts.Affectedness{Distance: 5}})
	})
}
