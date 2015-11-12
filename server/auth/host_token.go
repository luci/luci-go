// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"regexp"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/secrets"
	"github.com/luci/luci-go/server/tokens"
)

// hostToken describes how to validate X-Host-Token-V1 headers.
// See appengine/components/components/auth/host_token.py in luci-py repo.
var hostToken = tokens.TokenKind{
	Algo:       tokens.TokenAlgoHmacSHA256,
	Expiration: 24 * time.Hour, // host tokens have expiration time embedded
	SecretKey:  "host_token",   // name of the secret in AuthDB secrets bundle
	Version:    1,
}

// hostRe defines format of a string that can be embedded into the host token.
// Not very strict, just to ensure no fishy values are encoded into the token.
var hostRe = regexp.MustCompile(`^([a-z0-9\-]{1,40}\.?){1,10}$`)

// validateHostToken checks signature of X-Host-Token-V1 header and returns
// host name embedded in the token.
//
// Returns empty string (and logs error) if token is not valid. Returns error
// on transient errors (to trigger retry).
func validateHostToken(c context.Context, store secrets.Store, token string) (string, error) {
	tokenData, err := hostToken.Validate(secrets.Set(c, store), token, nil)
	if err != nil {
		if errors.IsTransient(err) {
			return "", err
		}
		logging.Errorf(c, "auth: was given invalid host token %q (%s), ignoring", token, err)
		return "", nil
	}

	host := tokenData["h"]
	if !hostRe.MatchString(host) {
		logging.Errorf(c, "auth: host %q in the host token is not valid, ignoring", host)
		return "", nil
	}

	return host, nil
}
