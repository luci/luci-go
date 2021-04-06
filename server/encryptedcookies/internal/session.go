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
	"fmt"
	"net/http"
	"strings"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/tink"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/encryptedcookies/internal/encryptedcookiespb"
	"go.chromium.org/luci/server/encryptedcookies/session"
)

const (
	// sessionCookieName is the name of the session cookie.
	sessionCookieName = "LUCISID"

	// rawCookiePrefix is prepended to the encrypted cookie value to give us
	// an ability to identify it in logs (if it leaks) and to version its
	// encryption/encoding scheme format.
	rawCookiePrefix = "lcsd_"

	// sessionCookieMaxAge is max-age of the session cookie.
	//
	// We do not expire *cookies* themselves. Session still can expire if the
	// refresh tokens backing them expire or get revoked. A session cookie
	// pointing to an expired session is ignored and opportunistically gets
	// removed.
	sessionCookieMaxAge = 60 * 60 * 24 * 365 * 20 // 20 years ~= infinity
)

// NewSessionCookie generates a new session cookie (in a clear text form).
//
// Generates the per-session encryption key and puts it into the produced
// cookies. Returns the AEAD primitive that can be used to encrypt things using
// the new per-session key.
func NewSessionCookie(id session.ID) (*encryptedcookiespb.SessionCookie, tink.AEAD) {
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		panic(fmt.Sprintf("could not generate session encryption key: %s", err))
	}

	// We'll encrypt the entire SessionCookie proto, so it is OK to use clear text
	// key there.
	buf := &bytes.Buffer{}
	if err := insecurecleartextkeyset.Write(kh, keyset.NewBinaryWriter(buf)); err != nil {
		panic(fmt.Sprintf("could not encrypt the session encryption key: %s", err))
	}

	// We just generated an AEAD keyset, it must be compatible with AEAD algo.
	a, err := aead.New(kh)
	if err != nil {
		panic(fmt.Sprintf("the keyset unexpectedly doesn't have AEAD primitive: %s", err))
	}

	return &encryptedcookiespb.SessionCookie{
		SessionId: id,
		Keyset:    buf.Bytes(),
	}, a
}

// EncryptSessionCookie produces the final session cookie.
func EncryptSessionCookie(aead tink.AEAD, pb *encryptedcookiespb.SessionCookie) (*http.Cookie, error) {
	enc, err := encryptB64(aead, pb, aeadContextSessionCookie)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    rawCookiePrefix + enc,
		Path:     "/",
		HttpOnly: true, // no access from Javascript
		MaxAge:   sessionCookieMaxAge,
	}, nil
}

// EncryptedSessionCookie finds the encrypted session cookie in the request.
//
// Returns nil if the cookie is not set.
func EncryptedSessionCookie(r *http.Request) *http.Cookie {
	cookie, _ := r.Cookie(sessionCookieName)
	return cookie
}

// DecryptSessionCookie decrypts the encrypted session cookie.
func DecryptSessionCookie(aead tink.AEAD, c *http.Cookie) (*encryptedcookiespb.SessionCookie, error) {
	if !strings.HasPrefix(c.Value, rawCookiePrefix) {
		return nil, errors.Reason("the value doesn't start with prefix %q", rawCookiePrefix).Err()
	}
	enc := strings.TrimPrefix(c.Value, rawCookiePrefix)
	cookie := &encryptedcookiespb.SessionCookie{}
	if err := decryptB64(aead, enc, cookie, aeadContextSessionCookie); err != nil {
		return nil, err
	}
	return cookie, nil
}

// PerSessionAEAD instantiates a tink.AEAD using the keyset from the cookie.
func PerSessionAEAD(c *encryptedcookiespb.SessionCookie) (tink.AEAD, error) {
	kh, err := insecurecleartextkeyset.Read(keyset.NewBinaryReader(bytes.NewReader(c.Keyset)))
	if err != nil {
		return nil, err
	}
	return aead.New(kh)
}
