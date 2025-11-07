// Copyright 2017 The LUCI Authors.
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

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/server/auth/internal"
	"go.chromium.org/luci/server/caching/layered"
)

var (
	// ErrBadOAuthToken is returned by GoogleOAuth2Method if the access token it
	// checks either totally invalid, expired or has a wrong list of scopes.
	ErrBadOAuthToken = grpcutil.UnauthenticatedTag.Apply(errors.New("oauth: bad access token"))

	// ErrBadAuthorizationHeader is returned by GoogleOAuth2Method if it doesn't
	// recognize the format of Authorization header.
	ErrBadAuthorizationHeader = grpcutil.UnauthenticatedTag.Apply(errors.New("oauth: bad Authorization header"))
)

// GoogleOAuth2Info is information extracted from Google OAuth2 access token.
//
// Available in case the request was authenticated using GoogleOAuth2Method. Use
// GetGoogleOAuth2Info(ctx) to grab GoogleOAuth2Info from within a request
// handler.
type GoogleOAuth2Info struct {
	// Scopes is a sorted list of OAuth scopes the token has.
	//
	// This is a superset of scopes that GoogleOAuth2Method checks. It can contain
	// extra scopes if the client chose to include them when obtaining the token.
	//
	// Do not modify.
	Scopes []string
}

// GetGoogleOAuth2Info returns information extracted from Google OAuth2 access
// token or nil if the request was not authenticated based on an access token.
func GetGoogleOAuth2Info(ctx context.Context) *GoogleOAuth2Info {
	info, _ := CurrentUser(ctx).Extra.(*GoogleOAuth2Info)
	return info
}

// tokenValidationOutcome is returned by validateAccessToken and cached in
// oauthValidationCache.
//
// It either contains an info extracted from the token or an error message if
// the token is invalid.
type tokenValidationOutcome struct {
	Email    string   `json:"email,omitempty"`
	ClientID string   `json:"client_id,omitempty"`
	Scopes   []string `json:"scopes,omitempty"` // sorted
	Expiry   int64    `json:"expiry,omitempty"` // unix timestamp
	Error    string   `json:"error,omitempty"`
}

// SHA256(access token) => JSON-marshalled *tokenValidationOutcome.
var oauthValidationCache = layered.RegisterCache(layered.Parameters[*tokenValidationOutcome]{
	ProcessCacheCapacity: 65536,
	GlobalNamespace:      "oauth_validation_v1",
	Marshal: func(item *tokenValidationOutcome) ([]byte, error) {
		return json.Marshal(item)
	},
	Unmarshal: func(blob []byte) (*tokenValidationOutcome, error) {
		tok := &tokenValidationOutcome{}
		if err := json.Unmarshal(blob, tok); err != nil {
			return nil, err
		}
		return tok, nil
	},
})

// GoogleOAuth2Method implements Method via Google's OAuth2 token info endpoint.
//
// Note that it uses the endpoint which "has no SLA and is not intended for
// production use". The closest alternative is /userinfo endpoint, but it
// doesn't return the token expiration time (so we can't cache the result of
// the check) nor the list of OAuth scopes the token has, nor the client ID to
// check against an allowlist.
//
// The general Google's recommendation is to use access tokens only for
// accessing Google APIs and use OpenID Connect Identity tokens for
// authentication in your own services instead (they are locally verifiable
// JWTs).
//
// Unfortunately, using OpenID tokens for LUCI services and OAuth2 access token
// for Google services significantly complicates clients, especially in
// non-trivial cases (like authenticating from a Swarming job): they now must
// support two token kinds and know which one to use when.
//
// There's no solution currently that preserves all of correctness, performance,
// usability and availability:
//   - Using /tokeninfo (like is done currently) sacrifices availability.
//   - Using /userinfo sacrifices correctness (no client ID or scopes check).
//   - Using OpenID ID tokens scarifies usability for the clients.
type GoogleOAuth2Method struct {
	// Scopes is a list of OAuth scopes to check when authenticating the token.
	Scopes []string

	// tokenInfoEndpoint is used in unit test to mock production endpoint.
	tokenInfoEndpoint string
}

var _ UserCredentialsGetter = (*GoogleOAuth2Method)(nil)

// Authenticate implements Method.
func (m *GoogleOAuth2Method) Authenticate(ctx context.Context, r RequestMetadata) (*User, Session, error) {
	// Extract the access token from the Authorization header.
	header := r.Header("Authorization")
	if header == "" || len(m.Scopes) == 0 {
		return nil, nil, nil // this method is not applicable
	}
	accessToken, err := accessTokenFromHeader(header)
	if err != nil {
		return nil, nil, err
	}

	// Store only the token hash in the cache, so that if a memory or cache dump
	// ever occurs, the tokens themselves aren't included in it.
	h := sha256.Sum256([]byte(accessToken))
	cacheKey := hex.EncodeToString(h[:])

	// Verify the token using /tokeninfo endpoint or grab a result of the previous
	// verification. We cache both good and bad tokens for extra 10 min to avoid
	// uselessly rechecking them all the time. Note that a bad token can't turn
	// into a good one with the passage of time, so its OK to cache it. And a good
	// token can turn into a bad one only when it expires (we check it below), so
	// it is also OK to cache it.
	//
	// TODO(vadimsh): Strictly speaking we need to store bad tokens in a separate
	// cache, so a flood of bad tokens (which are very easy to produce, compared
	// to good tokens) doesn't evict good tokens from the process cache.
	outcome, err := oauthValidationCache.GetOrCreate(ctx, cacheKey, func() (*tokenValidationOutcome, time.Duration, error) {
		logging.Infof(ctx, "oauth: validating access token SHA256=%q", cacheKey)
		outcome, expiresIn, err := validateAccessToken(ctx, accessToken, m.tokenInfoEndpoint)
		if err != nil {
			return nil, 0, err
		}
		return outcome, 10*time.Minute + expiresIn, nil
	})
	if err != nil {
		return nil, nil, err // the check itself failed
	}

	// Fail if the token was never valid.
	if outcome.Error != "" {
		logging.Warningf(ctx, "oauth: access token SHA256=%q: %s", cacheKey, outcome.Error)
		return nil, nil, ErrBadOAuthToken
	}

	// Fail if the token was once valid but has expired since.
	if expired := clock.Now(ctx).Unix() - outcome.Expiry; expired > 0 {
		logging.Warningf(ctx, "oauth: access token SHA256=%q from %s expired %d sec ago",
			cacheKey, outcome.Email, expired)
		return nil, nil, ErrBadOAuthToken
	}

	// Fail if the token doesn't have all required scopes.
	var missingScopes []string
	for _, s := range m.Scopes {
		idx := sort.SearchStrings(outcome.Scopes, s)
		if idx == len(outcome.Scopes) || outcome.Scopes[idx] != s {
			missingScopes = append(missingScopes, s)
		}
	}
	if len(missingScopes) != 0 {
		logging.Warningf(ctx, "oauth: access token SHA256=%q from %s doesn't have scopes %q, it has %q",
			cacheKey, outcome.Email, missingScopes, outcome.Scopes)
		return nil, nil, ErrBadOAuthToken
	}

	// OAuth2 access token representing service accounts have essentially
	// service account's uint64 user ID as an audience. It makes no sense to
	// check it against OAuth2 client ID allowlist (it will basically require us
	// to centrally allowlist every service account ever: we already use groups
	// with service account emails for that).
	if strings.HasSuffix(outcome.Email, ".gserviceaccount.com") {
		outcome.ClientID = ""
	}

	return &User{
		Identity: identity.Identity("user:" + outcome.Email),
		Email:    outcome.Email,
		ClientID: outcome.ClientID,
		Extra: &GoogleOAuth2Info{
			Scopes: outcome.Scopes,
		},
	}, nil, nil
}

// GetUserCredentials implements UserCredentialsGetter.
func (m *GoogleOAuth2Method) GetUserCredentials(ctx context.Context, r RequestMetadata) (*oauth2.Token, error) {
	accessToken, err := accessTokenFromHeader(r.Header("Authorization"))
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken: accessToken,
		TokenType:   "Bearer",
	}, nil
}

// accessTokenFromHeader parses Authorization header.
func accessTokenFromHeader(header string) (string, error) {
	typ, tok := internal.SplitAuthHeader(header)
	if typ != "bearer" && typ != "oauth" {
		return "", ErrBadAuthorizationHeader
	}
	return tok, nil
}

// validateAccessToken uses OAuth2 tokeninfo endpoint to validate an access
// token.
//
// Returns its outcome as tokenValidationOutcome. It either contains a token
// info or an error message if the token is invalid. If the token is valid,
// also returns the duration until it expires.
//
// Returns an error if the check itself fails, e.g. we couldn't make the
// request. Such errors may be transient (network flakes) or fatal
// (auth library misconfiguration).
func validateAccessToken(ctx context.Context, accessToken, tokenInfoEndpoint string) (*tokenValidationOutcome, time.Duration, error) {
	tr, err := GetRPCTransport(ctx, NoAuth)
	if err != nil {
		return nil, 0, err
	}

	tokenInfo, err := queryTokenInfoEndpoint(ctx, googleoauth.TokenInfoParams{
		AccessToken: accessToken,
		Client:      &http.Client{Transport: tr},
		Endpoint:    tokenInfoEndpoint, // "" means "use default"
	})
	if err != nil {
		if err == googleoauth.ErrBadToken {
			return &tokenValidationOutcome{Error: err.Error()}, 0, nil
		}
		return nil, 0, transient.Tag.Apply(errors.Fmt("oauth: transient error when validating the token: %w", err))
	}

	// Verify the token contains all necessary fields.
	errorMsg := ""
	switch {
	case tokenInfo.Email == "":
		errorMsg = "the token is not associated with an email"
	case !tokenInfo.EmailVerified:
		errorMsg = fmt.Sprintf("the email %s in the token is not verified", tokenInfo.Email)
	case tokenInfo.ExpiresIn <= 0:
		errorMsg = fmt.Sprintf("in a token from %s 'expires_in' %d is not a positive integer", tokenInfo.Email, tokenInfo.ExpiresIn)
	case tokenInfo.Aud == "":
		errorMsg = fmt.Sprintf("in a token from %s 'aud' field is empty", tokenInfo.Email)
	case tokenInfo.Scope == "":
		errorMsg = fmt.Sprintf("in a token from %s 'scope' field is empty", tokenInfo.Scope)
	}
	if errorMsg != "" {
		return &tokenValidationOutcome{Error: errorMsg}, 0, nil
	}

	// Verify the email passes our regexp check.
	if _, err := identity.MakeIdentity("user:" + tokenInfo.Email); err != nil {
		return &tokenValidationOutcome{Error: err.Error()}, 0, nil
	}

	// Sort scopes alphabetically to speed up lookups in Authenticate.
	scopes := strings.Split(tokenInfo.Scope, " ")
	sort.Strings(scopes)

	// The token is good.
	expiresIn := time.Duration(tokenInfo.ExpiresIn) * time.Second
	return &tokenValidationOutcome{
		Email:    tokenInfo.Email,
		ClientID: tokenInfo.Aud,
		Scopes:   scopes,
		Expiry:   clock.Now(ctx).Add(expiresIn).Unix(),
	}, expiresIn, nil
}

// queryTokenInfoEndpoint calls the token info endpoint with retries.
func queryTokenInfoEndpoint(ctx context.Context, params googleoauth.TokenInfoParams) (info *googleoauth.TokenInfo, err error) {
	ctx = clock.Tag(ctx, "oauth-tokeninfo-retry")

	retryParams := func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   10 * time.Millisecond,
				Retries: 5,
			},
		}
	}

	err = retry.Retry(ctx, transient.Only(retryParams), func() (err error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		start := clock.Now(ctx)
		outcome := "ERROR"

		switch info, err = googleoauth.GetTokenInfo(ctx, params); {
		case err == nil:
			outcome = "OK"
		case err == googleoauth.ErrBadToken:
			outcome = "BAD_TOKEN"
		case ctx.Err() != nil:
			outcome = "DEADLINE"
		}

		tokenInfoCallDuration.Add(ctx, float64(clock.Since(ctx, start).Nanoseconds()/1000), outcome)

		return err
	}, retry.LogCallback(ctx, "tokeninfo"))

	return info, err
}
