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

package gerritauth

import (
	"context"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/jwt"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/signing"
)

// Method is the auth.Method instance that checks Gerrit JWTs.
//
// It is initialized by the server module by default. Use it in your production
// code. In tests it is better to construct AuthMethod instances explicitly.
var Method AuthMethod

// AssertedInfo is information extracted from the JWT signed by Gerrit.
//
// JWTs are usually obtained by Gerrit frontend plugins when they want to make
// an external call on behalf of the Gerrit user. Information contained in JWTs
// identifies the Gerrit end-user (including all their linked Gerrit accounts)
// and the CL the plugin was operating in.
//
// Use GetAssertedInfo(ctx) to grab AssertedInfo from within a request handler.
type AssertedInfo struct {
	User   AssertedUser
	Change AssertedChange
}

// AssertedUser is part of the Gerrit JWT, it points to a Gerrit user.
type AssertedUser struct {
	AccountID      int64    `json:"account_id"`      // e.g. 1234, local to the Gerrit host
	Emails         []string `json:"emails"`          // list of all user emails
	PreferredEmail string   `json:"preferred_email"` // the email shown in the Gerrit UI
}

// AssertedChange is part of the Gerrit JWT, it points to a Gerrit CL.
type AssertedChange struct {
	Host         string `json:"host"`          // e.g. "chromium"
	Repository   string `json:"repository"`    // e.g. "infra/infra"
	ChangeNumber int64  `json:"change_number"` // e.g. 1254633
}

// GetAssertedInfo returns Gerrit CL and user info as asserted in the JWT.
//
// Works only from within a request handler and only if the call was
// authenticated via a Gerrit JWT. In all other cases (anonymous calls, calls
// authenticated via some other mechanism, etc.) returns nil.
func GetAssertedInfo(ctx context.Context) *AssertedInfo {
	info, _ := auth.CurrentUser(ctx).Extra.(*AssertedInfo)
	return info
}

// AuthMethod is an auth.Method implementation that checks Gerrit JWTs.
//
// On success puts *AssertedInfo into User.Extra field. Use GetAssertedInfo
// to access it.
type AuthMethod struct {
	// Header is a name of the request header to check for JWTs.
	Header string
	// SignerAccounts are emails of services account that sign Gerrit JWTs.
	SignerAccounts []string
	// Audience is an expected "aud" field of JWTs.
	Audience string

	testCerts *signing.PublicCertificates // for usage in tests
}

var _ interface {
	auth.Method
	auth.Warmable
} = (*AuthMethod)(nil)

// gerritJWT is a body of the JWT token produced by Gerrit.
type gerritJWT struct {
	Aud            string         `json:"aud"`
	Iss            string         `json:"iss"`
	Exp            int64          `json:"exp"`
	AssertedUser   AssertedUser   `json:"asserted_user"`
	AssertedChange AssertedChange `json:"asserted_change"`
}

// isConfigured is true if the method is fully configured and active.
func (m *AuthMethod) isConfigured() bool {
	return m.Header != "" && len(m.SignerAccounts) != 0
}

// Authenticate extracts user information from the incoming request.
//
// It is part of auth.Method interface.
func (m *AuthMethod) Authenticate(ctx context.Context, r auth.RequestMetadata) (*auth.User, auth.Session, error) {
	if !m.isConfigured() {
		return nil, nil, nil // skip, not configured
	}

	encodedJWT := r.Header(m.Header)
	if encodedJWT == "" {
		return nil, nil, nil // skip, no auth header
	}

	// Peek inside the token to see what account it was supposedly signed by.
	var unverifiedTok gerritJWT
	if err := jwt.UnsafeDecode(encodedJWT, &unverifiedTok); err != nil {
		return nil, nil, errors.Fmt("bad Gerrit JWT: %w", err)
	}

	// It must be one of the accounts we know.
	knownIssuer := ""
	for _, email := range m.SignerAccounts {
		if email == unverifiedTok.Iss {
			knownIssuer = email
			break
		}
	}
	if knownIssuer == "" {
		return nil, nil, errors.Fmt("bad Gerrit JWT: unrecognized issuer %q", unverifiedTok.Iss)
	}

	// Grab the signing keys we trust. Note: this usually hits the process cache.
	certs := m.testCerts
	if certs == nil {
		var err error
		certs, err = signing.FetchCertificatesForServiceAccount(ctx, knownIssuer)
		if err != nil {
			return nil, nil, errors.Fmt("could not fetch Gerrit public keys: %w", err)
		}
	}

	// Verify the signature and deserialize the token.
	var tok gerritJWT
	if err := jwt.VerifyAndDecode(encodedJWT, &tok, certs); err != nil {
		return nil, nil, errors.Fmt("bad Gerrit JWT: %w", err)
	}

	// Check the token was addressed to us.
	if tok.Aud != m.Audience {
		return nil, nil, errors.Fmt("bad Gerrit JWT: wrong audience %q, expecting %q", tok.Aud, m.Audience)
	}

	// Check the token expiration time. Allow 30 sec clock skew.
	now := clock.Now(ctx)
	exp := time.Unix(tok.Exp, 0)
	if exp.Add(30 * time.Second).Before(now) {
		return nil, nil, errors.Fmt("bad Gerrit JWT: expired %s ago", now.Sub(exp))
	}

	// Use "preferred_email", but fallback to "emails[0]" if empty, which
	// theoretically may happen if the preferred email is not backed by an
	// external ID.
	preferredEmail := tok.AssertedUser.PreferredEmail
	if preferredEmail == "" {
		if len(tok.AssertedUser.Emails) == 0 {
			return nil, nil, errors.New("bad Gerrit JWT: asserted_user.preferred_email and asserted_user.emails are empty")
		}
		preferredEmail = tok.AssertedUser.Emails[0]
	}

	// It must be syntactically a valid email address.
	ident, err := identity.MakeIdentity("user:" + preferredEmail)
	if err != nil {
		return nil, nil, errors.Fmt("bad Gerrit JWT: unrecognized email format: %w", err)
	}

	// Success.
	return &auth.User{
		Identity: ident,
		Email:    preferredEmail,
		Extra: &AssertedInfo{
			User:   tok.AssertedUser,
			Change: tok.AssertedChange,
		},
	}, nil, nil
}

// Warmup may be called to precache the data needed by the method.
//
// It is part of auth.Warmable interface.
func (m *AuthMethod) Warmup(ctx context.Context) error {
	if m.isConfigured() && m.testCerts == nil {
		var merr errors.MultiError
		for _, email := range m.SignerAccounts {
			_, err := signing.FetchCertificatesForServiceAccount(ctx, email)
			merr.MaybeAdd(err)
		}
		return merr.AsError()
	}
	return nil
}
