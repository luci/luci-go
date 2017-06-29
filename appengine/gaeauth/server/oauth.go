// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/urlfetch"
	"github.com/luci/gae/service/user"

	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/googleoauth"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/identity"
)

// EmailScope is a scope used to identifies user's email. Present in most tokens
// by default. Can be used as a base scope for authentication.
const EmailScope = "https://www.googleapis.com/auth/userinfo.email"

// OAuth2Method implements auth.Method on top of GAE OAuth2 API. It doesn't
// implement auth.UsersAPI.
type OAuth2Method struct {
	// Scopes is a list of OAuth scopes to check when authenticating the token.
	Scopes []string

	// tokenInfoEndpoint is used in unit test to mock production endpoint.
	tokenInfoEndpoint string
}

// Authenticate extracts peer's identity from the incoming request.
func (m *OAuth2Method) Authenticate(c context.Context, r *http.Request) (*auth.User, error) {
	header := r.Header.Get("Authorization")
	if header == "" || len(m.Scopes) == 0 {
		return nil, nil // this method is not applicable
	}
	if info.IsDevAppServer(c) {
		return m.authenticateDevServer(c, header, m.Scopes)
	}
	return m.authenticateProd(c, m.Scopes)
}

// authenticateProd is called on real appengine.
func (m *OAuth2Method) authenticateProd(c context.Context, scopes []string) (*auth.User, error) {
	// GetOAuthUser RPC is notoriously flaky. Do a bunch of retries on errors.
	var err error
	for attemp := 0; attemp < 4; attemp++ {
		var u *user.User
		u, err = user.CurrentOAuth(c, scopes...)
		if err != nil {
			logging.Warningf(c, "oauth: failed to execute GetOAuthUser - %s", err)
			continue
		}
		if u == nil {
			return nil, nil
		}
		if u.ClientID == "" {
			return nil, fmt.Errorf("oauth: ClientID is unexpectedly empty")
		}
		id, idErr := identity.MakeIdentity("user:" + u.Email)
		if idErr != nil {
			return nil, idErr
		}
		return &auth.User{
			Identity:  id,
			Superuser: u.Admin,
			Email:     u.Email,
			ClientID:  u.ClientID,
		}, nil
	}
	return nil, transient.Tag.Apply(err)
}

type tokenCheckCache string

// authenticateDevServer is called on dev server. It is using OAuth2 tokeninfo
// endpoint via URL fetch. It is slow as hell, but good enough for local manual
// testing.
func (m *OAuth2Method) authenticateDevServer(c context.Context, header string, scopes []string) (*auth.User, error) {
	chunks := strings.SplitN(header, " ", 2)
	if len(chunks) != 2 || (chunks[0] != "OAuth" && chunks[0] != "Bearer") {
		return nil, errors.New("oauth: bad Authorization header")
	}
	accessToken := chunks[1]

	// Maybe already verified this token?
	if user, ok := proccache.Get(c, tokenCheckCache(accessToken)); ok {
		return user.(*auth.User), nil
	}

	// Fetch an info dict associated with the token.
	logging.Infof(c, "oauth: Querying tokeninfo endpoint")
	tokenInfo, err := googleoauth.GetTokenInfo(c, googleoauth.TokenInfoParams{
		AccessToken: accessToken,
		Client:      &http.Client{Transport: urlfetch.Get(c)},
		Endpoint:    m.tokenInfoEndpoint,
	})
	if err != nil {
		if err == googleoauth.ErrBadToken {
			return nil, err
		}
		return nil, errors.Annotate(err, "oauth: transient error when validating token").
			Tag(transient.Tag).Err()
	}

	// Verify the token contains a validated email.
	switch {
	case tokenInfo.Email == "":
		return nil, fmt.Errorf("oauth: token is not associated with an email")
	case !tokenInfo.EmailVerified:
		return nil, fmt.Errorf("oauth: email %s is not verified", tokenInfo.Email)
	}
	if tokenInfo.ExpiresIn <= 0 {
		return nil, fmt.Errorf("oauth: 'expires_in' field is not a positive integer")
	}

	// Verify `scopes` is subset of tokenInfo.Scope.
	tokenScopes := map[string]bool{}
	for _, s := range strings.Split(tokenInfo.Scope, " ") {
		tokenScopes[s] = true
	}
	for _, s := range scopes {
		if !tokenScopes[s] {
			return nil, fmt.Errorf("oauth: token doesn't have scope %q", s)
		}
	}

	// Good enough.
	id, err := identity.MakeIdentity("user:" + tokenInfo.Email)
	if err != nil {
		return nil, err
	}
	u := &auth.User{
		Identity: id,
		Email:    tokenInfo.Email,
		ClientID: tokenInfo.Aud,
	}
	proccache.Put(c, tokenCheckCache(accessToken), u, time.Duration(tokenInfo.ExpiresIn)*time.Second)
	return u, nil
}
