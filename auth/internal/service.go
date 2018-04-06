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

package internal

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

type serviceAccountTokenProvider struct {
	ctx     context.Context // only for logging
	jsonKey []byte
	path    string
	scopes  []string
}

// NewServiceAccountTokenProvider returns TokenProvider that uses service
// account private key (on disk or in memory) to make access tokens.
func NewServiceAccountTokenProvider(ctx context.Context, jsonKey []byte, path string, scopes []string) (TokenProvider, error) {
	return &serviceAccountTokenProvider{
		ctx:     ctx,
		jsonKey: jsonKey,
		path:    path,
		scopes:  scopes,
	}, nil
}

func (p *serviceAccountTokenProvider) jwtConfig(ctx context.Context) (*jwt.Config, error) {
	jsonKey := p.jsonKey
	if p.path != "" {
		var err error
		logging.Debugf(ctx, "Reading private key from %s", p.path)
		jsonKey, err = ioutil.ReadFile(p.path)
		if err != nil {
			return nil, err
		}
	}
	return google.JWTConfigFromJSON(jsonKey, p.scopes...)
}

func (p *serviceAccountTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *serviceAccountTokenProvider) Lightweight() bool {
	return false
}

func (p *serviceAccountTokenProvider) Email() string {
	switch cfg, err := p.jwtConfig(p.ctx); {
	case err != nil:
		// Return UnknownEmail since we couldn't load it. This will trigger a code
		// path that attempts to refresh the token, where this error will be hit
		// again and properly reported.
		return UnknownEmail
	case cfg.Email == "":
		// Service account JSON file doesn't have 'email' field. Assume the email
		// is not available in that case. Strictly speaking we may try to generate
		// an OAuth token and then ask token info endpoint for an email, but this is
		// too much work. We require 'email' field to be present instead.
		return NoEmail
	default:
		return cfg.Email
	}
}

func (p *serviceAccountTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	cfg, err := p.jwtConfig(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to load private key JSON - %s", err)
		return nil, ErrBadCredentials
	}
	// PrivateKeyID is optional part of the private key JSON. If not given, use
	// a digest of the private key itself. This ID is used strictly locally, it
	// doesn't matter how we get it as long as it is repeatable between process
	// invocations.
	pkeyID := cfg.PrivateKeyID
	if pkeyID == "" {
		h := sha1.New()
		h.Write(cfg.PrivateKey)
		pkeyID = "custom:" + hex.EncodeToString(h.Sum(nil))
	}
	return &CacheKey{
		Key:    fmt.Sprintf("service_account/%s/%s", cfg.Email, pkeyID),
		Scopes: p.scopes,
	}, nil
}

func (p *serviceAccountTokenProvider) MintToken(ctx context.Context, base *Token) (*Token, error) {
	cfg, err := p.jwtConfig(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to load private key JSON - %s", err)
		return nil, ErrBadCredentials
	}
	switch newTok, err := grabToken(cfg.TokenSource(ctx)); {
	case err == nil:
		email := cfg.Email
		if email == "" {
			email = NoEmail
		}
		return &Token{
			Token: *newTok,
			Email: email,
		}, nil
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Error when creating access token - %s", err)
		return nil, err
	default:
		logging.Warningf(ctx, "Invalid or revoked service account key - %s", err)
		return nil, ErrBadCredentials
	}
}

func (p *serviceAccountTokenProvider) RefreshToken(ctx context.Context, prev, base *Token) (*Token, error) {
	// JWT tokens are self sufficient, there's no need for refresh_token. Minting
	// a token and "refreshing" it is a same thing.
	return p.MintToken(ctx, base)
}
