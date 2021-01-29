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

package artifactcontent

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	Convey("sizeBucket", t, func() {
		So(sizeBucket(-1), ShouldEqual, firstBucket)
		So(sizeBucket(0), ShouldEqual, firstBucket)
		So(sizeBucket(1000), ShouldBeLessThan, sizeBucket(1000*1000))
		So(sizeBucket(0x7FFFFFFFFFFFFFFF), ShouldBeGreaterThan, 0)
	})
}
