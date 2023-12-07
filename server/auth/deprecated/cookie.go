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

package deprecated

import (
	"context"
	"net/http"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/tokens"
)

// Note: this file is a part of deprecated CookieAuthMethod implementation.

// sessionCookieName is actual cookie name to set.
const sessionCookieName = "oid_session"

// sessionCookieToken is used to generate signed cookies that hold session ID.
var sessionCookieToken = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: 30 * 24 * time.Hour, // one month
	SecretKey:  "openid_session_cookie",
	Version:    1,
}

// makeSessionCookie takes a session ID and makes a signed cookie that can be
// put in a response.
func makeSessionCookie(ctx context.Context, sid string, secure bool) (*http.Cookie, error) {
	tok, err := sessionCookieToken.Generate(ctx, nil, map[string]string{"sid": sid}, 0)
	if err != nil {
		return nil, err
	}
	// Make cookie expire a bit earlier, to avoid weird "bad token" errors if
	// clocks are out of sync.
	exp := sessionCookieToken.Expiration - 15*time.Minute
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    tok,
		Path:     "/",
		Secure:   secure,
		HttpOnly: true, // no access from Javascript
		Expires:  clock.Now(ctx).Add(exp),
		MaxAge:   int(exp / time.Second),
	}, nil
}

// decodeSessionCookie takes an incoming request and returns a session ID stored
// in a session cookie, or "" if not there, invalid or expired. Returns errors
// on transient errors only.
func decodeSessionCookie(ctx context.Context, r interface {
	Cookie(string) (*http.Cookie, error)
}) (string, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", nil // no such cookie
	}
	payload, err := sessionCookieToken.Validate(ctx, cookie.Value, nil)
	switch {
	case transient.Tag.In(err):
		return "", err
	case err != nil:
		logging.Warningf(ctx, "Failed to decode session cookie %q: %s", cookie.Value, err)
		return "", nil
	case payload["sid"] == "":
		logging.Warningf(ctx, "No 'sid' key in cookie payload %v", payload)
		return "", nil
	}
	return payload["sid"], nil
}
