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

package internal

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/loginsessions/internal/statepb"
	"go.chromium.org/luci/server/secrets"
)

func TestRandom(t *testing.T) {
	t.Parallel()

	ftt.Run("RandomBlob", t, func(t *ftt.Test) {
		assert.Loosely(t, RandomBlob(40), should.HaveLength(40))
	})

	ftt.Run("RandomAlphaNum", t, func(t *ftt.Test) {
		assert.Loosely(t, RandomAlphaNum(40), should.HaveLength(40))
	})
}

func TestState(t *testing.T) {
	t.Parallel()

	ftt.Run("Round trip", t, func(t *ftt.Test) {
		ctx := secrets.GeneratePrimaryTinkAEADForTest(context.Background())

		original := &statepb.OpenIDState{
			LoginSessionId:   "login-session-id",
			LoginCookieValue: "cookie-value",
		}

		blob, err := EncryptState(ctx, original)
		assert.Loosely(t, err, should.BeNil)

		decrypted, err := DecryptState(ctx, blob)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, decrypted, should.Resemble(original))
	})

}
