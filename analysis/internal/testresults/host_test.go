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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestHost(t *testing.T) {
	ftt.Run("Gerrit hostnames", t, func(t *ftt.Test) {
		t.Run("Roundtrip", func(t *ftt.Test) {
			values := []string{
				"myproject-review.googlesource.com",
				"other123-review.googlesource.com",
				"something-other-123.gerrit.instance",
			}
			for _, input := range values {
				assert.Loosely(t, DecompressHost(CompressHost(input)), should.Equal(input))
			}
		})
		t.Run("Compress", func(t *ftt.Test) {
			testCases := map[string]string{
				// Should get some compression.
				"myproject-review.googlesource.com": "myproject",
				"some.other.instance":               "some.other.instance",
			}
			for input, expectedOutput := range testCases {
				assert.Loosely(t, CompressHost(input), should.Equal(expectedOutput))
			}
		})
		t.Run("Decompress", func(t *ftt.Test) {
			testCases := map[string]string{
				"myproject":           "myproject-review.googlesource.com",
				"some.other.instance": "some.other.instance",
			}
			for input, expectedOutput := range testCases {
				assert.Loosely(t, DecompressHost(input), should.Equal(expectedOutput))
			}
		})
	})
}
