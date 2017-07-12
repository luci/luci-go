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

package cmpbin

import (
	"bytes"
	"io"
	"math/rand"
	"sort"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFloats(t *testing.T) {
	t.Parallel()

	Convey("floats", t, func() {
		b := &bytes.Buffer{}

		Convey("good", func() {
			f1 := float64(1.234)
			n, err := WriteFloat64(b, f1)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 8)
			f, n, err := ReadFloat64(b)
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 8)
			So(f, ShouldEqual, f1)
		})

		Convey("bad", func() {
			_, n, err := ReadFloat64(b)
			So(err, ShouldEqual, io.EOF)
			So(n, ShouldEqual, 0)
		})
	})
}

func TestFloatSortability(t *testing.T) {
	t.Parallel()

	Convey("floats maintain sort order", t, func() {
		vec := make(sort.Float64Slice, randomTestSize)
		r := rand.New(rand.NewSource(*seed))
		for i := range vec {
			vec[i] = r.Float64()
		}

		bin := make(sort.StringSlice, len(vec))
		b := &bytes.Buffer{}
		for i := range bin {
			b.Reset()
			n, err := WriteFloat64(b, vec[i])
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 8)
			bin[i] = b.String()
		}

		vec.Sort()
		bin.Sort()

		for i := range vec {
			r, _, err := ReadFloat64(bytes.NewBufferString(bin[i]))
			So(err, ShouldBeNil)
			So(vec[i], ShouldEqual, r)
		}
	})
}
