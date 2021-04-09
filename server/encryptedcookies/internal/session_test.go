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

	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSession(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
		So(err, ShouldBeNil)
		primaryAEAD, err := aead.New(kh)
		So(err, ShouldBeNil)

		cookie, sessionAEAD := NewSessionCookie(session.GenerateID())
		httpCookie, err := EncryptSessionCookie(primaryAEAD, cookie)
		So(err, ShouldBeNil)

		enc, err := EncryptPrivate(sessionAEAD, &sessionpb.Private{RefreshToken: "blah"})
		So(err, ShouldBeNil)

		req, _ := http.NewRequest("GET", "http://example.com", nil)
		req.Header.Add("Cookie", httpCookie.String())

		httpCookieFromReq, _ := req.Cookie(SessionCookieName)
		So(httpCookieFromReq, ShouldNotBeNil)

		cookieFromReq, err := DecryptSessionCookie(primaryAEAD, httpCookieFromReq)
		So(err, ShouldBeNil)

		priv, _, err := UnsealPrivate(cookieFromReq, &sessionpb.Session{
			EncryptedPrivate: enc,
		})
		So(err, ShouldBeNil)

		So(priv.RefreshToken, ShouldEqual, "blah")
	})
}
