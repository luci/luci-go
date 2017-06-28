// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry/transient"
)

type serviceAccountTokenProvider struct {
	jsonKey []byte
	path    string
	scopes  []string
}

// NewServiceAccountTokenProvider returns TokenProvider that uses service
// account private key (on disk or in memory) to make access tokens.
func NewServiceAccountTokenProvider(ctx context.Context, jsonKey []byte, path string, scopes []string) (TokenProvider, error) {
	return &serviceAccountTokenProvider{
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

func (p *serviceAccountTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	cfg, err := p.jwtConfig(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to load private key JSON - %s", err)
		return nil, err
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

func (p *serviceAccountTokenProvider) MintToken(ctx context.Context, base *oauth2.Token) (*oauth2.Token, error) {
	cfg, err := p.jwtConfig(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to load private key JSON - %s", err)
		return nil, ErrBadCredentials
	}
	switch newTok, err := grabToken(cfg.TokenSource(ctx)); {
	case err == nil:
		return newTok, nil
	case transient.Tag.In(err):
		logging.Warningf(ctx, "Error when creating access token - %s", err)
		return nil, err
	default:
		logging.Warningf(ctx, "Invalid or revoked service account key - %s", err)
		return nil, ErrBadCredentials
	}
}

func (p *serviceAccountTokenProvider) RefreshToken(ctx context.Context, prev, base *oauth2.Token) (*oauth2.Token, error) {
	// JWT tokens are self sufficient, there's no need for refresh_token. Minting
	// a token and "refreshing" it is a same thing.
	return p.MintToken(ctx, base)
}
