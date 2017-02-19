// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/auth/localauth/rpcs"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/lucictx"
)

type luciContextTokenProvider struct {
	localAuth *lucictx.LocalAuth
	scopes    []string
	transport http.RoundTripper
	cacheKey  CacheKey // used only for in-memory cache
}

// NewLUCIContextTokenProvider returns TokenProvider that knows how to use a
// local auth server to mint tokens.
//
// It requires LUCI_CONTEXT["local_auth"] to be present in the 'ctx'. It's a
// description of how to locate and contact the local auth server.
//
// See common/auth/localauth package for the implementation of the server.
func NewLUCIContextTokenProvider(ctx context.Context, scopes []string, transport http.RoundTripper) (TokenProvider, error) {
	localAuth := lucictx.GetLocalAuth(ctx)
	if localAuth == nil {
		return nil, fmt.Errorf(`no "local_auth" in LUCI_CONTEXT`)
	}

	// All authenticators share singleton in-process token cache, see
	// ProcTokenCache variable in proc_cache.go.
	//
	// It is possible (though very unusual), for a single process to use multiple
	// local auth servers (e.g if it enters a subcontext with another "local_auth"
	// value).
	//
	// For these reasons we use a digest of localAuth parameters as a cache key.
	// It is used only in the process-local cache, the token never ends up in
	// the disk cache, as indicated by Lightweight() returning true (tokens from
	// such providers aren't cached on disk by Authenticator).
	blob, err := json.Marshal(localAuth)
	if err != nil {
		return nil, err
	}
	digest := sha1.Sum(blob)

	return &luciContextTokenProvider{
		localAuth: localAuth,
		scopes:    scopes,
		transport: transport,
		cacheKey: CacheKey{
			Key:    fmt.Sprintf("luci_ctx/%s", hex.EncodeToString(digest[:])),
			Scopes: scopes,
		},
	}, nil
}

func (p *luciContextTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *luciContextTokenProvider) Lightweight() bool {
	return true
}

func (p *luciContextTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *luciContextTokenProvider) MintToken(ctx context.Context, base *oauth2.Token) (*oauth2.Token, error) {
	// Note: deadlines and retries are implemented by Authenticator. MintToken
	// should just make a single attempt, and mark an error as transient to
	// trigger a retry, if necessary.
	request := rpcs.GetOAuthTokenRequest{
		Scopes: p.scopes,
		Secret: p.localAuth.Secret,
	}
	if err := request.Validate(); err != nil {
		return nil, err // should not really happen
	}
	body, err := json.Marshal(&request)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/rpc/LuciLocalAuthService.GetOAuthToken", p.localAuth.RPCPort)
	logging.Debugf(ctx, "POST %s", url)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := ctxhttp.Do(ctx, &http.Client{Transport: p.transport}, httpReq)
	if err != nil {
		return nil, errors.WrapTransient(err)
	}
	defer httpResp.Body.Close()
	respBody, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return nil, errors.WrapTransient(err)
	}

	if httpResp.StatusCode != 200 {
		err := fmt.Errorf("local auth - HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
		if httpResp.StatusCode >= 500 {
			return nil, errors.WrapTransient(err)
		}
		return nil, err
	}

	response := rpcs.GetOAuthTokenResponse{}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, err
	}
	if response.ErrorMessage != "" || response.ErrorCode != 0 {
		msg := response.ErrorMessage
		if msg == "" {
			msg = "unknown error"
		}
		return nil, fmt.Errorf("local auth - RPC code %d: %s", response.ErrorCode, msg)
	}

	return &oauth2.Token{
		AccessToken: response.AccessToken,
		Expiry:      time.Unix(response.Expiry, 0).UTC(),
		TokenType:   "Bearer",
	}, nil
}

func (p *luciContextTokenProvider) RefreshToken(ctx context.Context, prev, base *oauth2.Token) (*oauth2.Token, error) {
	// Minting and refreshing is the same thing: a call to local auth server.
	return p.MintToken(ctx, base)
}
