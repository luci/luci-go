// Copyright 2026 The LUCI Authors.
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
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTokenFingerprint(t *testing.T) {
	t.Parallel()

	ftt.Run("TokenFingerprint", t, func(t *ftt.Test) {
		raw := "some-token-string"
		digest := sha256.Sum256([]byte(raw))
		expected := hex.EncodeToString(digest[:16])
		assert.Loosely(t, TokenFingerprint(raw), should.Equal(expected))
	})
}
