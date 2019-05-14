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

package utils

import (
	"math/rand"
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
			ShouldBeNil(err)
			ShouldBeGreaterThanOrEqualTo(1000, len(data))

			unpackedArray, err := Unpack(data)
			ShouldBeNil(err)
			ShouldEqual(array, unpackedArray)
		})

		Convey(`1m 1000`, func() {
			var array []int64
			for i := 0; i < 1000000; i++ {
				array = append(array, int64(i*1000))
			}

			data, err := Pack(array)
			ShouldBeNil(err)
			ShouldBeGreaterThanOrEqualTo(2000, len(data))

			unpackedArray, err := Unpack(data)
			ShouldBeNil(err)
			ShouldEqual(array, unpackedArray)
		})

		Convey(`1m pseudo`, func() {
			var array []int64
			r := rand.New(rand.NewSource(0))
			for i := 0; i < 1000000; i++ {
				array = append(array, r.Int63n(1000000))
			}

			data, err := Pack(array)
			ShouldBeNil(err)
			ShouldBeGreaterThanOrEqualTo(2000, len(data))

			unpackedArray, err := Unpack(data)
			ShouldBeNil(err)
			ShouldEqual(array, unpackedArray)
		})

		Convey(`empty`, func() {
			data, err := Pack([]int64{})
			ShouldBeNil(err)
			ShouldBeEmpty(data)

			unpackedArray, err := Unpack([]byte{})
			ShouldBeNil(err)
			ShouldBeEmpty(unpackedArray)
		})
	})
}
