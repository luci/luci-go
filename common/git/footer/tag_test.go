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

package footer

import (
	"testing"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseLegacyMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseLegacyMetadata", t, func(t *ftt.Test) {
		t.Run("Complete", func(t *ftt.Test) {
			actual := ParseLegacyMetadata(`message

commit details...
MD_ONE=1111
more details...

MD-HYPHEN=invalid
MD_lowercase=invalid
Footer: Cool
MD_TWO=2222
Footer: Awesome
`)
			assert.Loosely(t, actual, should.Resemble(strpair.ParseMap([]string{
				"MD_ONE:1111",
				"MD_TWO:2222",
			})))
		})
		t.Run("Honor metadata order", func(t *ftt.Test) {
			actual := ParseLegacyMetadata(`MD_FOO=foo
MD_FOO=bar
MD_FOO=baz
`)
			assert.Loosely(t, actual["MD_FOO"], should.Resemble([]string{
				"baz",
				"bar",
				"foo",
			}))
		})
	})
}
