// Copyright 2016 The LUCI Authors.
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

package prpc

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFormat(t *testing.T) {
	ftt.Run("requestFormat", t, func(t *ftt.Test) {
		test := func(contentType string, expectedFormat Format, expectedErr any) {
			t.Run("Content-Type: "+contentType, func(t *ftt.Test) {
				actualFormat, err := FormatFromContentType(contentType)
				assert.Loosely(t, err, should.ErrLike(expectedErr))
				if err == nil {
					assert.Loosely(t, actualFormat, should.Equal(expectedFormat))
				}
			})
		}

		test("", FormatBinary, nil)
		test(ContentTypePRPC, FormatBinary, nil)
		test(mtPRPCBinary, FormatBinary, nil)
		test(mtPRPCJSONPBLegacy, FormatJSONPB, nil)
		test(mtPRPCText, FormatText, nil)
		test(
			ContentTypePRPC+"; encoding=blah",
			0,
			`invalid encoding parameter: "blah". Valid values: "binary", "json", "text"`)
		test(ContentTypePRPC+"; boo=true", 0, `unexpected parameter "boo"`)

		test(ContentTypeJSON, FormatJSONPB, nil)
		test(ContentTypeJSON+"; whatever=true", FormatJSONPB, nil)

		test("x", 0, `unknown content type: "x"`)
		test("x,y", 0, "mime: expected slash after first token")
	})
}
