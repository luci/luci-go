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

package distribution

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFixedWidthBucketer(t *testing.T) {
	Convey("Invalid values panic", t, func() {
		So(func() { FixedWidthBucketer(10, -1) }, ShouldPanic)
		So(func() { FixedWidthBucketer(-1, 1) }, ShouldPanic)
	})

	Convey("One size", t, func() {
		b := FixedWidthBucketer(10, 1)
		So(b.NumBuckets(), ShouldEqual, 3)
		So(b.Bucket(-100), ShouldEqual, 0)
		So(b.Bucket(-1), ShouldEqual, 0)
		So(b.Bucket(0), ShouldEqual, 1)
		So(b.Bucket(5), ShouldEqual, 1)
		So(b.Bucket(10), ShouldEqual, 2)
		So(b.Bucket(100), ShouldEqual, 2)
	})
}

func TestGeometricBucketer(t *testing.T) {
	Convey("Invalid values panic", t, func() {
		So(func() { GeometricBucketer(10, -1) }, ShouldPanic)
		So(func() { GeometricBucketer(-1, 10) }, ShouldPanic)
		So(func() { GeometricBucketer(0, 10) }, ShouldPanic)
		So(func() { GeometricBucketer(1, 10) }, ShouldPanic)
		So(func() { GeometricBucketer(0.5, 10) }, ShouldPanic)
	})

	Convey("One size", t, func() {
		b := GeometricBucketer(4, 4)
		So(b.NumBuckets(), ShouldEqual, 6)
		So(b.Bucket(-100), ShouldEqual, 0)
		So(b.Bucket(-1), ShouldEqual, 0)
		So(b.Bucket(0), ShouldEqual, 0)
		So(b.Bucket(1), ShouldEqual, 1)
		So(b.Bucket(3), ShouldEqual, 1)
		So(b.Bucket(4), ShouldEqual, 2)
		So(b.Bucket(15), ShouldEqual, 2)
		So(b.Bucket(16), ShouldEqual, 3)
		So(b.Bucket(63), ShouldEqual, 3)
		So(b.Bucket(64), ShouldEqual, 4)
	})
}

func TestGeometricBucketerWithScale(t *testing.T) {
	Convey("Invalid values panic", t, func() {
		So(func() { GeometricBucketerWithScale(10, 10, -1.0) }, ShouldPanic)
		So(func() { GeometricBucketerWithScale(10, 10, 0) }, ShouldPanic)
	})

	Convey("pass", t, func() {
		b := GeometricBucketerWithScale(4, 4, 100)
		So(b.NumBuckets(), ShouldEqual, 6)
		So(b.Bucket(-100), ShouldEqual, 0)
		So(b.Bucket(-1), ShouldEqual, 0)
		So(b.Bucket(0), ShouldEqual, 0)
		So(b.Bucket(100), ShouldEqual, 1)
		So(b.Bucket(300), ShouldEqual, 1)
		So(b.Bucket(400), ShouldEqual, 2)
		So(b.Bucket(1500), ShouldEqual, 2)
		So(b.Bucket(1600), ShouldEqual, 3)
		So(b.Bucket(6300), ShouldEqual, 3)
		So(b.Bucket(6400), ShouldEqual, 4)
	})
}
