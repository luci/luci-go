// Copyright 2025 The LUCI Authors.
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
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"golang.org/x/net/publicsuffix"

	"go.chromium.org/luci/auth/reauth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

const metadataKeyRapt = "rapt"

// A ReAuthenticator wraps a normal [Authenticator] with ReAuth support.
type ReAuthenticator struct {
	*Authenticator

	// Alternative for testing
	provider raptProvider
}

type raptProvider func(context.Context, *http.Client) (*reauth.RAPT, error)

// NewReAuthenticator creates a new [ReAuthenticator].
func NewReAuthenticator(a *Authenticator) *ReAuthenticator {
	return &ReAuthenticator{
		Authenticator: a,
	}
}

// GetRAPT returns a ReAuth proof token if available and valid.
//
// Does not interact with the user. May return ErrLoginRequired.
func (a *ReAuthenticator) GetRAPT(ctx context.Context) (string, error) {
	var rapt reauth.RAPT
	if err := a.UnpackTokenMetadata(metadataKeyRapt, &rapt); err != nil {
		return "", errors.Annotate(err, "get RAPT").Err()
	}
	if rapt.Token == "" {
		return "", errors.Reason("get RAPT: missing RAPT").Err()
	}
	// Add some buffer to the expiration so it can be used by the caller.
	if clock.Now(ctx).After(rapt.Expiry.Add(-5 * time.Minute)) {
		return "", errors.Reason("get RAPT: expired RAPT").Err()
	}

	return rapt.Token, nil
}

// RenewRAPT renews the RAPT for the current OAuth token.
//
// This is always interactive and only works for human users.
func (a *ReAuthenticator) RenewRAPT(ctx context.Context) error {
	// Use the embedded client since RAPT is not needed to renew RAPT.
	c, err := a.Authenticator.Client() //nolint: staticcheck
	if err != nil {
		return errors.Annotate(err, "renew RAPT").Err()
	}
	r, err := a.mintRAPT(ctx, c)
	if err != nil {
		return errors.Annotate(err, "renew RAPT").Err()
	}
	if err := a.PackTokenMetadata(ctx, metadataKeyRapt, r); err != nil {
		return errors.Annotate(err, "renew RAPT").Err()
	}
	return nil
}

// Client optionally performs a login and returns http.Client.
//
// Compared to [Authenticator], this method attaches a RAPT, which
// must be renewed separately with user interaction (see
// [ReAuthenticator.RenewRAPT]).
func (a *ReAuthenticator) Client(ctx context.Context) (*http.Client, error) {
	c, err := a.Authenticator.Client()
	if err != nil {
		return nil, errors.Annotate(err, "ReAuthenticator.Client").Err()
	}
	if c.Jar == nil {
		j, err := cookiejar.New(&cookiejar.Options{
			PublicSuffixList: publicsuffix.List,
		})
		if err != nil {
			panic(err)
		}
		c.Jar = j
	}
	rapt, err := a.GetRAPT(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "ReAuthenticator.Client").Err()
	}
	c.Jar.SetCookies(
		&url.URL{
			Scheme: "https",
			Host:   "googlesource.com",
		},
		[]*http.Cookie{{
			Name:   "RAPT",
			Value:  rapt,
			Domain: "googlesource.com",
			Secure: true,
			// TODO(ayatane): We should set MaxAge here, but
			// expired RAPT and missing RAPT are identical.
		}})
	return c, nil
}

func (a *ReAuthenticator) mintRAPT(ctx context.Context, c *http.Client) (*reauth.RAPT, error) {
	if a.provider != nil {
		return a.provider(ctx, c)
	}
	return reauth.GetRAPT(ctx, c)
}
