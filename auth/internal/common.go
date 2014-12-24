// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package internal contains interfaces and structs used internally
by infra.libs.auth. They are moved to a separate package for a better
readability of main infra.libs.auth code: infra.libs.auth implements top level
logic, infra.libs.auth.internal implements dirty details.
*/
package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"code.google.com/p/goauth2/oauth"
)

// Token is immutable object that internally holds authentication credentials
// (short term, long term, or both). Token knows how to modify http.Request to
// apply the credentials to it. TokenProvider acts as a factory for Tokens.
type Token interface {
	// Equals compares this token to another token of the same kind.
	Equals(Token) bool

	// RequestHeaders returns a map of authorization headers to add to the request.
	RequestHeaders() map[string]string

	// Expired is true if token MUST be refreshed via RefreshToken call.
	Expired() bool
}

// TokenProvider knows how to mint new tokens, refresh existing ones, marshal
// and umarshal tokens to byte buffer. TokenProvider acts as a factory for
// particular implementations of Token interface. Tokens from two different
// provider instances must not be mixed together.
type TokenProvider interface {
	// RequiresInteraction is true if provider may start user interaction in MintToken.
	RequiresInteraction() bool

	// MintToken launches authentication flow (possibly interactive) and returns
	// a new refreshable token. It uses provided round tripper to make requests.
	MintToken(http.RoundTripper) (Token, error)

	// RefreshToken takes existing token (probably expired, but not necessarily)
	// and returns a new refreshed token. It should never do any user interaction.
	// If a user interaction is required, a error should be returned instead. The
	// function uses provided round tripper to make requests.
	RefreshToken(Token, http.RoundTripper) (Token, error)

	// MarshalToken converts a token to byte buffer.
	MarshalToken(Token) ([]byte, error)

	// UnmarshalToken takes byte buffer produced by MarshalToken and returns
	// original Token.
	UnmarshalToken([]byte) (Token, error)
}

///////////////////////////////////////////////////////////////////////////////

// tokenImpl implements Token interface by adapting oauth.Token.
type tokenImpl struct {
	oauth.Token
}

// makeToken builds Token from oauth.Token by copying it.
func makeToken(tok *oauth.Token) Token {
	return &tokenImpl{
		Token: *tok,
	}
}

// extractOAuthToken takes Token, checks its type and return oauth.Token.
// It returns a copy that can be safely mutated.
func extractOAuthToken(tok Token) oauth.Token {
	// It's OK to panic here on type mismatch.
	return tok.(*tokenImpl).Token
}

func (t *tokenImpl) Equals(another Token) bool {
	if another == nil {
		return false
	}
	casted, ok := another.(*tokenImpl)
	if !ok {
		return false
	}
	return t.AccessToken == casted.AccessToken
}

func (t *tokenImpl) RequestHeaders() map[string]string {
	ret := make(map[string]string)
	if t.AccessToken != "" {
		ret["Authorization"] = "Bearer " + t.AccessToken
	}
	return ret
}

///////////////////////////////////////////////////////////////////////////////

// oauthTokenProvider partially implements a TokenProvider built on top of oauth
// library. Concrete implementations embed this struct and provide missing
// MintToken and RefreshToken methods.
type oauthTokenProvider struct {
	interactive bool
	tokenFlavor string
}

type tokenOnDisk struct {
	Version      string `json:"version"`
	Flavor       string `json:"flavor"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAtSec int64  `json:"expires_at,omitempty"`
}

const tokFormatVersion = "1"

func (p *oauthTokenProvider) RequiresInteraction() bool {
	return p.interactive
}

func (p *oauthTokenProvider) MarshalToken(t Token) ([]byte, error) {
	tok := extractOAuthToken(t)
	return json.Marshal(&tokenOnDisk{
		Version:      tokFormatVersion,
		Flavor:       p.tokenFlavor,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAtSec: tok.Expiry.Unix(),
	})
}

func (p *oauthTokenProvider) UnmarshalToken(data []byte) (Token, error) {
	onDisk := tokenOnDisk{}
	if err := json.Unmarshal(data, &onDisk); err != nil {
		return nil, err
	}
	if onDisk.Version != tokFormatVersion {
		return nil, fmt.Errorf("auth: bad token version %q, expected %q", onDisk.Version, tokFormatVersion)
	}
	if onDisk.Flavor != p.tokenFlavor {
		return nil, fmt.Errorf("auth: bad token flavor %q, expected %q", onDisk.Flavor, p.tokenFlavor)
	}
	return makeToken(&oauth.Token{
		AccessToken:  onDisk.AccessToken,
		RefreshToken: onDisk.RefreshToken,
		Expiry:       time.Unix(onDisk.ExpiresAtSec, 0),
	}), nil
}
