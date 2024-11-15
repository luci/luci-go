// Copyright 2024 The LUCI Authors.
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

package botsession

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/hmactoken"
)

func TestMarshaling(t *testing.T) {
	t.Parallel()

	secret := hmactoken.NewStaticSecret(secrets.Secret{
		Active: []byte("secret"),
	})

	session := &internalspb.Session{
		BotId:     "bot-id",
		SessionId: "session-id",
	}

	t.Run("Round trip", func(t *testing.T) {
		blob, err := Marshal(session, secret)
		assert.That(t, err, should.ErrLike(nil))

		restored, err := Unmarshal(blob, secret)
		assert.That(t, err, should.ErrLike(nil))

		assert.That(t, session, should.Match(restored))
	})

	t.Run("Bad MAC", func(t *testing.T) {
		blob, err := Marshal(session, secret)
		assert.That(t, err, should.ErrLike(nil))

		anotherSecret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("another-secret"),
		})

		_, err = Unmarshal(blob, anotherSecret)
		assert.That(t, err, should.ErrLike("bad session token MAC"))
	})
}
