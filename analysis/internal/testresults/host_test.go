// Copyright 2022 The LUCI Authors.
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

package testresults

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHost(t *testing.T) {
	Convey("Gerrit hostnames", t, func() {
		Convey("Roundtrip", func() {
			values := []string{
				"myproject-review.googlesource.com",
				"other123-review.googlesource.com",
				"something-other-123.gerrit.instance",
			}
			for _, input := range values {
				So(DecompressHost(compressHost(input)), ShouldEqual, input)
			}
		})
		Convey("Compress", func() {
			testCases := map[string]string{
				// Should get some compression.
				"myproject-review.googlesource.com": "myproject",
				"some.other.instance":               "some.other.instance",
			}
			for input, expectedOutput := range testCases {
				So(compressHost(input), ShouldEqual, expectedOutput)
			}
		})
		Convey("Decompress", func() {
			testCases := map[string]string{
				"myproject":           "myproject-review.googlesource.com",
				"some.other.instance": "some.other.instance",
			}
			for input, expectedOutput := range testCases {
				So(DecompressHost(input), ShouldEqual, expectedOutput)
			}
		})
	})
}
