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
	"net/http"
	"testing"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"
)

func TestSession(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		assert.Loosely(t, err, should.BeNil)
		primaryAEAD, err := aead.New(kh)
		assert.Loosely(t, err, should.BeNil)

		cookie, sessionAEAD := NewSessionCookie(session.GenerateID())
		httpCookie, err := EncryptSessionCookie(primaryAEAD, cookie)
		assert.Loosely(t, err, should.BeNil)

		enc, err := EncryptPrivate(sessionAEAD, &sessionpb.Private{RefreshToken: "blah"})
		assert.Loosely(t, err, should.BeNil)

		req, _ := http.NewRequest("GET", "http://example.com", nil)
		req.Header.Add("Cookie", httpCookie.String())

		httpCookieFromReq, _ := req.Cookie(SessionCookieName)
		assert.Loosely(t, httpCookieFromReq, should.NotBeNil)

		cookieFromReq, err := DecryptSessionCookie(primaryAEAD, httpCookieFromReq)
		assert.Loosely(t, err, should.BeNil)

		priv, _, err := UnsealPrivate(cookieFromReq, &sessionpb.Session{
			EncryptedPrivate: enc,
		})
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, priv.RefreshToken, should.Equal("blah"))
	})
}
