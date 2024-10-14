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

package internal

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPKCE(t *testing.T) {
	t.Parallel()

	ftt.Run("GenerateCodeVerifier", t, func(t *ftt.Test) {
		verifier := GenerateCodeVerifier()
		assert.Loosely(t, verifier, should.HaveLength(50))
		for _, ch := range verifier {
			assert.Loosely(t, ch >= 'a' && ch <= 'z' ||
				ch >= 'A' && ch <= 'Z' ||
				ch >= '0' && ch <= '9' ||
				ch == '-' || ch == '.', should.BeTrue)
		}
	})

	ftt.Run("DeriveCodeChallenge", t, func(t *ftt.Test) {
		assert.Loosely(t, DeriveCodeChallenge("abc"), should.Equal("ungWv48Bz-pBQUDeXa4iI7ADYaOWF3qctBD_YfIAFa0"))
	})
}
