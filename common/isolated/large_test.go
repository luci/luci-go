// Copyright 2019 The LUCI Authors.
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
	"math/rand"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPack(t *testing.T) {
	t.Parallel()
	Convey(`Simple test for pack.`, t, func() {
		Convey(`1m 1`, func() {
			var array []int64
			for i := 0; i < 1000000; i++ {
				array = append(array, int64(i))
			}

			data, err := Pack(array)
			So(err, ShouldBeNil)
			So(1000, ShouldBeGreaterThanOrEqualTo, len(data))

			unpackedArray, err := Unpack(data)
			So(err, ShouldBeNil)
			So(array, ShouldResemble, unpackedArray)
		})

		Convey(`1m 1000`, func() {
			var array []int64
			for i := 0; i < 1000000; i++ {
				array = append(array, int64(i*1000))
			}

			data, err := Pack(array)
			So(err, ShouldBeNil)
			So(len(data), ShouldBeGreaterThan, 1000)

			unpackedArray, err := Unpack(data)
			So(err, ShouldBeNil)
			So(array, ShouldResemble, unpackedArray)
		})

		Convey(`1m pseudo`, func() {
			var array []int64
			r := rand.New(rand.NewSource(0))
			for i := 0; i < 1000000; i++ {
				array = append(array, r.Int63n(1000000))
			}
			sort.Slice(array, func(i, j int) bool {
				return array[i] <= array[j]
			})

			data, err := Pack(array)
			So(err, ShouldBeNil)
			So(len(data), ShouldBeGreaterThan, 2000)

			unpackedArray, err := Unpack(data)
			So(err, ShouldBeNil)
			So(array, ShouldResemble, unpackedArray)
		})

		Convey(`empty`, func() {
			data, err := Pack([]int64{})
			So(err, ShouldBeNil)
			So(data, ShouldBeEmpty)

			unpackedArray, err := Unpack([]byte{})
			So(err, ShouldBeNil)
			So(unpackedArray, ShouldBeEmpty)
		})
	})
}
