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
	"bytes"
	"testing"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/encryptedcookies/internal/encryptedcookiespb"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"
)

func TestGenerateNonce(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		nonce1 := GenerateNonce()
		nonce2 := GenerateNonce()
		assert.Loosely(t, nonce1, should.HaveLength(16))
		assert.Loosely(t, bytes.Equal(nonce1, nonce2), should.BeFalse)
	})
}

func TestCrypto(t *testing.T) {
	t.Parallel()

	ftt.Run("With keyset", t, func(t *ftt.Test) {
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		assert.Loosely(t, err, should.BeNil)
		ae, err := aead.New(kh)
		assert.Loosely(t, err, should.BeNil)

		t.Run("State enc/dec", func(t *ftt.Test) {
			state := &encryptedcookiespb.OpenIDState{DestHost: "blah"}

			enc, err := EncryptStateB64(ae, state)
			assert.Loosely(t, err, should.BeNil)

			dec, err := DecryptStateB64(ae, enc)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dec, should.Resemble(state))

			_, err = DecryptStateB64(ae, "aaaaaaaa"+enc[8:])
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Private enc/dec", func(t *ftt.Test) {
			priv := &sessionpb.Private{AccessToken: "blah"}

			enc, err := EncryptPrivate(ae, priv)
			assert.Loosely(t, err, should.BeNil)

			dec, err := DecryptPrivate(ae, enc)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dec, should.Resemble(priv))

			for i := 0; i < 8; i++ {
				enc[i] = 0
			}
			_, err = DecryptPrivate(ae, enc)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
