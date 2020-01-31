// Copyright 2020 The LUCI Authors.
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

package usercontent

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tokens"
)

var pathTokenKind = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: time.Hour,
	SecretKey:  "user_content",
	Version:    1,
}

// generateSignedURL generates a signed HTTPS URL back to this server.
// The token works only with the urlPath, after calling path.Clean.
// The function can be used for any kind of user content URLs.
func (s *Server) generateSignedURL(ctx context.Context, urlPath string) (u *url.URL, expiration time.Time, err error) {
	urlPath = path.Clean(urlPath)

	const ttl = time.Hour
	now := clock.Now(ctx).UTC()

	state := []byte(urlPath)
	tok, err := pathTokenKind.Generate(ctx, state, nil, ttl)
	if err != nil {
		return nil, time.Time{}, err
	}

	q := url.Values{}
	q.Set("token", tok)
	u = &url.URL{
		Scheme:   "https",
		Host:     s.Hostname,
		Path:     urlPath,
		RawQuery: q.Encode(),
	}
	if s.InsecureURLs {
		u.Scheme = "http"
	}
	expiration = now.Add(ttl)
	return
}

// validateToken validates token query parameter.
// It can be used as a router middleware.
func validateToken(ctx *router.Context, next router.Handler) {
	token := ctx.Request.URL.Query().Get("token")
	if token == "" {
		http.Error(ctx.Writer, "missing token query parameters", http.StatusUnauthorized)
		return
	}

	state := []byte(path.Clean(ctx.Request.URL.Path))
	if _, err := pathTokenKind.Validate(ctx.Context, token, state); err != nil {
		logging.Warningf(ctx.Context, "Token validation failed: %s", err)
		if transient.Tag.In(err) {
			http.Error(ctx.Writer, "Internal server error", http.StatusInternalServerError)
		} else {
			http.Error(ctx.Writer, "invalid token", http.StatusForbidden)
		}
		return
	}

	next(ctx)
}
