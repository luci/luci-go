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

package internal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/integration/localauth/rpcs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/lucictx"
)

type luciContextTokenProvider struct {
	localAuth *lucictx.LocalAuth
	email     string // an email or NoEmail
	scopes    []string
	audience  string // not empty iff using ID tokens
	transport http.RoundTripper
	cacheKey  CacheKey // used only for in-memory cache
}

// NewLUCIContextTokenProvider returns TokenProvider that knows how to use a
// local auth server to mint tokens.
//
// It requires LUCI_CONTEXT["local_auth"] to be present in the 'ctx'. It's a
// description of how to locate and contact the local auth server.
//
// See auth/integration/localauth package for the implementation of the server.
func NewLUCIContextTokenProvider(ctx context.Context, scopes []string, audience string, transport http.RoundTripper) (TokenProvider, error) {
	localAuth := lucictx.GetLocalAuth(ctx)
	switch {
	case localAuth == nil:
		return nil, fmt.Errorf(`no "local_auth" in LUCI_CONTEXT`)
	case localAuth.DefaultAccountId == "":
		return nil, fmt.Errorf(`no "default_account_id" in LUCI_CONTEXT["local_auth"]`)
	}

	// Grab an email associated with default account, if any.
	email := NoEmail
	for _, account := range localAuth.Accounts {
		if account.Id == localAuth.DefaultAccountId {
			// Previous protocol version didn't expose the email, so keep the value
			// as NoEmail in this case. This should be rare.
			if account.Email != "" {
				email = account.Email
			}
			break
		}
	}

	// All authenticators share singleton in-process token cache, see
	// ProcTokenCache variable in proc_cache.go.
	//
	// It is possible (though very unusual), for a single process to use multiple
	// local auth servers (e.g. if it enters a subcontext with another
	// "local_auth" value).
	//
	// For these reasons we use a digest of localAuth parameters as a cache key.
	// It is used only in the process-local cache, the token never ends up in
	// the disk cache, as indicated by MemoryCacheOnly() returning true.
	blob, err := json.Marshal(localAuth)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(blob)

	return &luciContextTokenProvider{
		localAuth: localAuth,
		email:     email,
		scopes:    scopes,
		audience:  audience,
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

func (p *luciContextTokenProvider) MemoryCacheOnly() bool {
	return true
}

func (p *luciContextTokenProvider) Email() string {
	return p.email
}

func (p *luciContextTokenProvider) CacheKey(ctx context.Context) (*CacheKey, error) {
	return &p.cacheKey, nil
}

func (p *luciContextTokenProvider) MintToken(ctx context.Context, base *Token) (*Token, error) {
	if p.audience == "" {
		return p.mintOAuthToken(ctx)
	}
	return p.mintIDToken(ctx)
}

func (p *luciContextTokenProvider) mintOAuthToken(ctx context.Context) (*Token, error) {
	request := &rpcs.GetOAuthTokenRequest{
		BaseRequest: rpcs.BaseRequest{
			Secret:    p.localAuth.Secret,
			AccountID: p.localAuth.DefaultAccountId,
		},
		Scopes: p.scopes,
	}
	response := &rpcs.GetOAuthTokenResponse{}
	if err := p.doRPC(ctx, "GetOAuthToken", request, response); err != nil {
		return nil, err
	}
	if err := p.handleRPCErr(&response.BaseResponse); err != nil {
		return nil, err
	}
	return &Token{
		Token: oauth2.Token{
			AccessToken: response.AccessToken,
			Expiry:      time.Unix(response.Expiry, 0).UTC(),
			TokenType:   "Bearer",
		},
		IDToken: NoIDToken,
		Email:   p.Email(),
	}, nil
}

func (p *luciContextTokenProvider) mintIDToken(ctx context.Context) (*Token, error) {
	request := &rpcs.GetIDTokenRequest{
		BaseRequest: rpcs.BaseRequest{
			Secret:    p.localAuth.Secret,
			AccountID: p.localAuth.DefaultAccountId,
		},
		Audience: p.audience,
	}
	response := &rpcs.GetIDTokenResponse{}
	if err := p.doRPC(ctx, "GetIDToken", request, response); err != nil {
		return nil, err
	}
	if err := p.handleRPCErr(&response.BaseResponse); err != nil {
		return nil, err
	}
	return &Token{
		Token: oauth2.Token{
			AccessToken: NoAccessToken,
			Expiry:      time.Unix(response.Expiry, 0).UTC(),
			TokenType:   "Bearer",
		},
		IDToken: response.IDToken,
		Email:   p.Email(),
	}, nil
}

func (p *luciContextTokenProvider) RefreshToken(ctx context.Context, prev, base *Token) (*Token, error) {
	// Minting and refreshing is the same thing: a call to a local auth server.
	return p.MintToken(ctx, base)
}

// doRPC sends a request to the local auth server and parses the response.
//
// Note: deadlines and retries are implemented by Authenticator. doRPC should
// just make a single attempt, and mark an error as transient to trigger a
// retry, if necessary.
func (p *luciContextTokenProvider) doRPC(ctx context.Context, method string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("http://127.0.0.1:%d/rpc/LuciLocalAuthService.%s", p.localAuth.RpcPort, method)
	logging.Debugf(ctx, "POST %s", url)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := ctxhttp.Do(ctx, &http.Client{Transport: p.transport}, httpReq)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	defer httpResp.Body.Close()
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	if httpResp.StatusCode != 200 {
		err := fmt.Errorf("local auth - HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
		if httpResp.StatusCode >= 500 {
			return transient.Tag.Apply(err)
		}
		return err
	}

	return json.Unmarshal(respBody, resp)
}

// handleRPCErr handles `error_message` and `error_code` response fields.
func (p *luciContextTokenProvider) handleRPCErr(resp *rpcs.BaseResponse) error {
	if resp.ErrorMessage != "" || resp.ErrorCode != 0 {
		msg := resp.ErrorMessage
		if msg == "" {
			msg = "unknown error"
		}
		return fmt.Errorf("local auth - RPC code %d: %s", resp.ErrorCode, msg)
	}
	return nil
}
