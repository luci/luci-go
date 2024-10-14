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

package coordinator

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLogStreamEncoding(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing log stream key encode/decode`, t, func(t *ftt.Test) {

		t.Run(`Will encode "quux" into "Key_cXV1eA~~".`, func(t *ftt.Test) {
			// Note that we test a key whose length is not a multiple of 3 so that we
			// can assert that the padding is correct, too.
			enc := encodeKey("quux")
			assert.Loosely(t, enc, should.Equal("Key_cXV1eA~~"))
		})

		for _, s := range []string{
			"",
			"hello",
			"from the outside",
			"+-#$!? \t\n",
		} {
			t.Run(fmt.Sprintf(`Can encode: %q`, s), func(t *ftt.Test) {
				enc := encodeKey(s)
				assert.Loosely(t, enc, should.HavePrefix(encodedKeyPrefix))

				t.Run(`And then decode back into the original string.`, func(t *ftt.Test) {
					dec, err := decodeKey(enc)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, dec, should.Equal(s))
				})
			})
		}

		t.Run(`Will fail to decode a string that doesn't begin with the encoded prefix.`, func(t *ftt.Test) {
			_, err := decodeKey("foo")
			assert.Loosely(t, err, should.ErrLike("encoded key missing prefix"))
		})

		t.Run(`Will fail to decode a string that's not properly encoded.`, func(t *ftt.Test) {
			_, err := decodeKey(encodedKeyPrefix + "!!!")
			assert.Loosely(t, err, should.ErrLike("failed to decode key"))
		})
	})
}
