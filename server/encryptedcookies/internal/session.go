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
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/tink"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/encryptedcookies/internal/encryptedcookiespb"
	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"
)

const (
	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "LUCISID"

	// UnlimitedCookiePath is a path to set the cookie on by default.
	UnlimitedCookiePath = "/"

	// LimitedCookiePath is a path to set the cookie on when limiting exposure.
	LimitedCookiePath = "/auth/openid/"

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

	// refreshMaxTTL defines what sessions are considered "definitely fresh".
	//
	// If a session's TTL (time till next NextRefresh) is longer than this, we
	// won't try to refresh it. Passing this threshold enables the probabilistic
	// early expiration check. See ShouldRefreshSession().
	//
	// We assume the typical session TTL period to be 1h.
	refreshMaxTTL = 20 * time.Minute

	// refreshMinTTL defines what sessions needs to be refreshed ASAP.
	//
	// If a session's TTL (time till next NextRefresh) is smaller than this, we
	// *must* refresh it.
	//
	// E.g. if the session *really* expires in 1h, we must be refreshing it after
	// >50 min TTL mark. This is needed to make sure short-lived tokens are usable
	// within the request handler (as long as its runtime is under 10 min). We
	// don't refresh tokens mid-way through the handler.
	//
	// The session is also probabilistically refreshed sooner if its TTL is less
	// than refreshMaxTTL. This avoids a potential stampede from parallel URL
	// Fetch browser calls when the actual deadline comes.
	refreshMinTTL = 10 * time.Minute

	// refreshExpMean is a parameter of the exponential distribution for
	// the probabilistic early expiration check.
	refreshExpMean = time.Minute
)

// NewSessionCookie generates a new session cookie (in a clear text form).
//
// Generates the per-session encryption key and puts it into the produced
// cookie. Returns the AEAD primitive that can be used to encrypt things using
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

// EncryptSessionCookie produces the session cookie with prepopulated fields.
//
// The caller still needs to fill in at least `Path` field.
func EncryptSessionCookie(aead tink.AEAD, pb *encryptedcookiespb.SessionCookie) (*http.Cookie, error) {
	enc, err := encryptB64(aead, pb, aeadContextSessionCookie)
	if err != nil {
		return nil, err
	}
	return &http.Cookie{
		Name:     SessionCookieName,
		Value:    rawCookiePrefix + enc,
		HttpOnly: true, // no access from Javascript
		MaxAge:   sessionCookieMaxAge,
	}, nil
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

// UnsealPrivate decrypts the private part of the session using the key from
// the cookie.
//
// Returns the instantiated per-session AEAD primitive.
func UnsealPrivate(c *encryptedcookiespb.SessionCookie, s *sessionpb.Session) (*sessionpb.Private, tink.AEAD, error) {
	kh, err := insecurecleartextkeyset.Read(keyset.NewBinaryReader(bytes.NewReader(c.Keyset)))
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to deserialize per-session keyset").Err()
	}
	ae, err := aead.New(kh)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to instantiate per-session AEAD").Err()
	}
	private, err := DecryptPrivate(ae, s.EncryptedPrivate)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to decrypt the private part of the session").Err()
	}
	return private, ae, nil
}

// ShouldRefreshSession returns true if we should refresh the session now.
//
// The decision in based on `ttl`, which is a duration till the hard session
// staleness deadline. We attempt to refresh the session sooner.
func ShouldRefreshSession(ctx context.Context, ttl time.Duration) bool {
	switch {
	case ttl > refreshMaxTTL:
		return false // too soon to refresh
	case ttl < refreshMinTTL:
		return true // definitely need to refresh
	default:
		threshold := time.Duration(mathrand.ExpFloat64(ctx) * float64(refreshExpMean))
		return ttl-refreshMinTTL < threshold
	}
}
